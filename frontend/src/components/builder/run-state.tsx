"use client";

import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";

import { executions, watchExecution, type ExecutionStatus } from "@/lib/api";

/**
 * What the canvas knows about the run it is watching.
 *
 * Deliberately not stored on the nodes. The nodes array is what autosave
 * serialises, so a status merged into node.data would be written into the
 * stored graph — a run's outcome becoming part of the workflow's definition.
 * A context keeps the two apart with no discipline required at the call site.
 */
type RunState = {
  executionId: string | null;
  status: ExecutionStatus | null;
  nodes: Record<string, ExecutionStatus>;
  error: string | null;
  /** Per-node failure messages, so a node can show why it failed. */
  nodeErrors: Record<string, string>;
};

const empty: RunState = {
  executionId: null,
  status: null,
  nodes: {},
  error: null,
  nodeErrors: {},
};

const RunContext = createContext<RunState>(empty);

export const useRunState = () => useContext(RunContext);
export const useNodeStatus = (nodeId: string) => useContext(RunContext).nodes[nodeId];

export function RunProvider({
  workspaceId,
  executionId,
  children,
}: {
  workspaceId: string;
  executionId: string | null;
  children: React.ReactNode;
}) {
  const [state, setState] = useState<RunState>(empty);
  // The run this provider has already wired up, so a re-render does not open a
  // second socket for the same execution.
  const watching = useRef<string | null>(null);

  const apply = useCallback((next: Partial<RunState>) => {
    setState((current) => ({ ...current, ...next }));
  }, []);

  useEffect(() => {
    if (!executionId || watching.current === executionId) return;
    watching.current = executionId;

    let close: (() => void) | undefined;
    let live = true;

    (async () => {
      setState({ ...empty, executionId });

      // Connect first. This is the contract: nothing is replayed, so a run that
      // ends between the fetch and the subscribe would leave the canvas showing
      // RUNNING forever.
      close = await watchExecution(workspaceId, executionId, (event) => {
        if (!live) return;
        setState((current) => {
          if (event.type === "node" && event.nodeId) {
            return {
              ...current,
              nodes: { ...current.nodes, [event.nodeId]: event.status },
              nodeErrors: event.error
                ? { ...current.nodeErrors, [event.nodeId]: event.error }
                : current.nodeErrors,
            };
          }
          return { ...current, status: event.status, error: event.error ?? current.error };
        });
      });

      // Then reconcile. A fast run can be over before the socket opened, and
      // this is what fills the canvas in when that happens. Events that arrive
      // after it simply overwrite the same keys.
      try {
        const snapshot = await executions.get(workspaceId, executionId);
        if (!live) return;
        const nodes: Record<string, ExecutionStatus> = {};
        const nodeErrors: Record<string, string> = {};
        for (const node of snapshot.nodes) {
          nodes[node.nodeId] = node.status;
          if (node.error) nodeErrors[node.nodeId] = node.error;
        }
        setState((current) => ({
          ...current,
          status: snapshot.execution.status,
          error: snapshot.execution.error ?? current.error,
          // The snapshot goes underneath: anything the socket has already
          // delivered is newer than a fetch that was in flight beside it.
          nodes: { ...nodes, ...current.nodes },
          nodeErrors: { ...nodeErrors, ...current.nodeErrors },
        }));
      } catch {
        // The socket is the live path; a failed reconcile is not worth
        // surfacing on its own.
      }
    })();

    return () => {
      live = false;
      close?.();
    };
  }, [workspaceId, executionId, apply]);

  return <RunContext.Provider value={state}>{children}</RunContext.Provider>;
}
