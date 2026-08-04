"use client";

import { createContext, useCallback, useContext, useEffect, useState } from "react";

import {
  executions,
  isTerminal,
  watchExecution,
  type Execution,
  type ExecutionDetail,
  type ExecutionStatus,
  type Graph,
  type NodeRun,
} from "@/lib/api";

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
  /**
   * What each node returned. A failed node keeps its output on purpose — an
   * HTTP node rejected with a 401 still holds the body explaining why, and
   * that body is the whole reason anyone opens a failed run.
   */
  nodeOutputs: Record<string, unknown>;
  /**
   * The run row and the graph it ran, once the snapshot has landed. No event
   * carries either — the socket announces transitions, not the record — so
   * these stay null until the fetch returns and are the run viewer's material.
   */
  execution: Execution | null;
  graph: Graph | null;
  /**
   * Per-node timing, which also only exists on the row: a duration is computed
   * from two timestamps, and a node that is still running has one of them.
   */
  nodeTimes: Record<string, Pick<NodeRun, "startedAt" | "finishedAt" | "durationMs">>;
};

const empty: RunState = {
  executionId: null,
  status: null,
  nodes: {},
  error: null,
  nodeErrors: {},
  nodeOutputs: {},
  execution: null,
  graph: null,
  nodeTimes: {},
};

const RunContext = createContext<RunState>(empty);

export const useRunState = () => useContext(RunContext);
export const useNodeStatus = (nodeId: string) => useContext(RunContext).nodes[nodeId];

/** What one node did: its status, its output, and why it failed. */
export function useNodeResult(nodeId: string) {
  const run = useContext(RunContext);
  return {
    status: run.nodes[nodeId],
    output: run.nodeOutputs[nodeId],
    error: run.nodeErrors[nodeId],
  };
}

/**
 * Fold a fetched snapshot into what the socket has already delivered.
 *
 * Which side wins is per field rather than wholesale, and that is the whole
 * content of this function: statuses and outputs can arrive either way, so the
 * live one is preferred; timings and the run row only ever come from the fetch,
 * so it is the only answer there is.
 */
function merged(current: RunState, snapshot: ExecutionDetail): RunState {
  const nodes: Record<string, ExecutionStatus> = {};
  const nodeErrors: Record<string, string> = {};
  const nodeOutputs: Record<string, unknown> = {};
  const nodeTimes: RunState["nodeTimes"] = {};

  for (const node of snapshot.nodes) {
    nodes[node.nodeId] = node.status;
    if (node.error) nodeErrors[node.nodeId] = node.error;
    if (node.output !== undefined) nodeOutputs[node.nodeId] = node.output;
    nodeTimes[node.nodeId] = {
      startedAt: node.startedAt,
      finishedAt: node.finishedAt,
      durationMs: node.durationMs,
    };
  }

  return {
    ...current,
    execution: snapshot.execution,
    graph: snapshot.graph,
    // A terminal status is never walked back. If the run ended while this fetch
    // was in flight, the socket has already said so and the snapshot it raced
    // is the older answer — taking it would leave the page reading RUNNING with
    // no event left to correct it, which is the exact failure connect-before-
    // fetch exists to prevent.
    status: isTerminal(current.status) ? current.status : snapshot.execution.status,
    error: snapshot.execution.error ?? current.error,
    // The snapshot goes underneath: anything the socket has already delivered
    // is newer than a fetch that was in flight beside it.
    nodes: { ...nodes, ...current.nodes },
    nodeErrors: { ...nodeErrors, ...current.nodeErrors },
    nodeOutputs: { ...nodeOutputs, ...current.nodeOutputs },
    nodeTimes,
  };
}

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

  const apply = useCallback((next: Partial<RunState>) => {
    setState((current) => ({ ...current, ...next }));
  }, []);

  // No "have I already subscribed to this id" ref guarding this effect, and
  // that is deliberate. The dependency array is the guard: it re-runs only when
  // the execution changes. A ref on top of it survives an unmount, so React's
  // development mount -> cleanup -> mount makes the second mount skip its own
  // subscribe while the first mount's fetch is already being discarded as
  // stale. That leaves the page on its skeleton with a 200 in the network tab,
  // which is precisely what it did.
  useEffect(() => {
    if (!executionId) return;

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
              nodeOutputs:
                event.output === undefined
                  ? current.nodeOutputs
                  : { ...current.nodeOutputs, [event.nodeId]: event.output },
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
        if (live) setState((current) => merged(current, snapshot));
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

  // One more fetch when the run stops.
  //
  // Durations are the reason: they are computed from the row's two timestamps,
  // and the finishing event announces a status rather than a record. Without
  // this a run watched from start to end shows every outcome and no timing,
  // while the same run opened afterwards shows both — the viewer would
  // disagree with itself depending on when you arrived.
  const finished = isTerminal(state.status);
  useEffect(() => {
    if (!executionId || !finished) return;
    let live = true;
    executions
      .get(workspaceId, executionId)
      .then((snapshot) => {
        if (live) setState((current) => merged(current, snapshot));
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [workspaceId, executionId, finished]);

  return <RunContext.Provider value={state}>{children}</RunContext.Provider>;
}
