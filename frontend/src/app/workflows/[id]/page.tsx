"use client";

import {
  Background,
  Controls,
  ReactFlow,
  addEdge,
  useEdgesState,
  useNodesState,
  type Connection,
  type Edge,
  type Node,
  type OnEdgesChange,
  type OnNodesChange,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import Link from "next/link";
import { useParams } from "next/navigation";
import { ArrowLeft, Copy, History, MoreHorizontal, Play, Plus, SlidersHorizontal } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { nodeTypes } from "@/components/builder/node";
import { Inspector, Palette, SaveState } from "@/components/builder/panels";
import { RunProvider, useRunState } from "@/components/builder/run-state";
import { RunStatus } from "@/components/run-status";
import { Modal } from "@/components/modal";
import { Button, ErrorNote, Skeleton, Wordmark } from "@/components/ui";
import {
  apiError,
  genericWebhookURL,
  githubWebhookURL,
  workflows,
  type Graph,
  type Workflow as WorkflowRecord,
  type WorkflowNode,
} from "@/lib/api";
import { specFor } from "@/lib/node-types";
import {
  useCancelExecution,
  useCreateTemplate,
  useExecutions,
  useStartExecution,
  useWorkflow,
  useRestoreWorkflow,
  useWorkflowVersions,
  useWorkspace,
} from "@/lib/queries";

/**
 * The builder.
 *
 * React Flow's node and edge shape is exactly what the API stores, so there is
 * no mapping layer — `toGraph` below is a strip, not a translation, and that
 * distinction matters: the moment it starts renaming fields, the canvas and the
 * engine have two representations that can drift.
 */
export default function BuilderPage() {
  const { id } = useParams<{ id: string }>();
  const { workspace } = useWorkspace();
  const { data: workflow, isPending, error } = useWorkflow(workspace?.id, id);

  if (error) {
    return (
      <div className="mx-auto max-w-md p-10">
        <ErrorNote>{apiError(error)}</ErrorNote>
      </div>
    );
  }
  if (isPending || !workflow || !workspace) return <BuilderSkeleton />;

  // Keyed on the workflow, so the canvas below can seed its state from props at
  // mount rather than in an effect. Seeding in an effect means a render with an
  // empty canvas first, and a second setState before anything is painted.
  return <Builder key={workflow.id} workspaceId={workspace.id} workflow={workflow} />;
}

function Builder({
  workspaceId,
  workflow,
}: {
  workspaceId: string;
  workflow: WorkflowRecord;
}) {
  const id = workflow.id;

  // Initialised once, from the graph as it was fetched. A later refetch does
  // not re-seed: it would throw away whatever is being dragged at that moment.
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>(
    workflow.graph.nodes as unknown as Node[],
  );
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>(
    workflow.graph.edges as unknown as Edge[],
  );

  const graph = useMemo(() => toGraph(nodes, edges), [nodes, edges]);
  const { saving, saveError, dirty } = useAutosave(workspaceId, id, graph);

  const selected = nodes.find((node) => node.selected);
  const start = useStartExecution(workspaceId);
  const restore = useRestoreWorkflow(workspaceId, id);
  const versions = useWorkflowVersions(workspaceId, id);
  const createTemplate = useCreateTemplate(workspaceId);
  const cancel = useCancelExecution(workspaceId);
  const { data: runs } = useExecutions(workspaceId);
  const lastRun = runs?.find((run) => run.workflowId === id);
  const githubTriggered = nodes.some((node) => node.type === "trigger.github.pull_request");
  const webhookTriggered = nodes.some((node) => node.type === "trigger.webhook");

  // The run this canvas is watching. Set when Run is pressed, and picked up
  // from the newest run on load so reopening a workflow mid-run shows it.
  const [watchingId, setWatchingId] = useState<string | null>(null);
  const [showHistory, setShowHistory] = useState(false);
  const [showTemplate, setShowTemplate] = useState(false);
  const [webhookCopied, setWebhookCopied] = useState(false);
  const [mobilePanel, setMobilePanel] = useState<"steps" | "inspector" | null>(null);
  const runId = watchingId ?? (lastRun && lastRun.status === "RUNNING" ? lastRun.id : null);
  const hookURL = githubTriggered
    ? githubWebhookURL(id)
    : webhookTriggered
      ? genericWebhookURL(id)
      : null;

  const addNode = useCallback(
    (type: string) => {
      setNodes((current) => {
        // Placed to the right of whatever is furthest right, so a new node
        // never lands on top of an existing one.
        const x = current.length === 0 ? 80 : Math.max(...current.map((n) => n.position.x)) + 260;
        return [
          ...current.map((node) => ({ ...node, selected: false })),
          {
            id: nextId(type, current),
            type,
            position: { x, y: 160 },
            data: { label: specFor(type)?.label ?? type },
            selected: true,
          } as Node,
        ];
      });
    },
    [setNodes],
  );

  const updateData = useCallback(
    (nodeId: string, data: Record<string, unknown>) =>
      setNodes((current) =>
        current.map((node) => (node.id === nodeId ? { ...node, data } : node)),
      ),
    [setNodes],
  );

  const deleteNode = useCallback(
    (nodeId: string) => {
      setNodes((current) => current.filter((node) => node.id !== nodeId));
      // Edges touching a deleted node go with it. The backend would reject a
      // graph whose edge ends nowhere, and it would be right to.
      setEdges((current) =>
        current.filter((edge) => edge.source !== nodeId && edge.target !== nodeId),
      );
    },
    [setNodes, setEdges],
  );

  return (
    <div className="flex h-screen flex-col bg-cream-wash">
      <header className="flex h-[62px] shrink-0 items-center gap-2 border-b border-hairline bg-canvas-white px-3 sm:gap-3 sm:px-4">
        <Link href="/workflows" className="flex items-center gap-2 text-ash hover:text-ink">
          <ArrowLeft size={16} strokeWidth={1.75} />
          <Wordmark className="hidden xl:inline-flex" />
        </Link>

        <span className="min-w-0 flex-1 truncate text-body-sm text-ink sm:ml-1">
          {workflow.name}
        </span>

        <span className={saveError ? "max-w-24 truncate" : "hidden min-[430px]:inline"} title={saveError ?? undefined}><SaveState saving={saving} error={saveError} dirty={dirty} /></span>
        {lastRun && <RunStatus status={lastRun.status} className="hidden md:inline-flex" />}

        <div className="hidden items-center gap-2 lg:flex">
        {hookURL && (
          <Button
            variant="neutral"
            className="h-9"
            title={hookURL}
            onClick={() => {
              void navigator.clipboard
                .writeText(hookURL)
                .then(() => setWebhookCopied(true))
                .catch(() => setWebhookCopied(false));
            }}
          >
            <Copy size={14} strokeWidth={1.75} />
            <span className="hidden sm:inline">{webhookCopied ? "Copied" : "Copy webhook URL"}</span>
          </Button>
        )}

        <Button variant="neutral" className="h-9" onClick={() => setShowHistory(true)}>
          <History size={14} strokeWidth={1.75} />
          <span className="hidden sm:inline">History</span>
        </Button>

        <Button variant="neutral" className="h-9" onClick={() => setShowTemplate(true)}>
          Save template
        </Button>

        {lastRun?.status === "RUNNING" && (
          // Cancel reaches only runs this process is executing. One that
          // finished a moment ago answers 409, which is honest rather than an
          // error worth showing.
          <Button
            variant="neutral"
            className="h-9"
            disabled={cancel.isPending}
            onClick={() => cancel.mutate(lastRun.id)}
          >
            {cancel.isPending ? "Cancelling…" : "Cancel"}
          </Button>
        )}
        </div>

        <details className="group relative lg:hidden">
          <summary aria-label="More workflow actions" className="flex size-9 cursor-pointer list-none items-center justify-center rounded-lg bg-pearl text-ink">
            <MoreHorizontal size={17} strokeWidth={1.75} />
          </summary>
          <div className="absolute right-0 top-11 z-50 flex w-52 flex-col gap-1 rounded-xl border border-hairline bg-canvas-white p-2 shadow-raised">
            {hookURL && (
              <button className="flex items-center gap-2 rounded-lg px-3 py-2 text-left text-body-sm hover:bg-pearl" onClick={() => void navigator.clipboard.writeText(hookURL).then(() => setWebhookCopied(true)).catch(() => setWebhookCopied(false))}>
                <Copy size={14} strokeWidth={1.75} /> {webhookCopied ? "Webhook copied" : "Copy webhook URL"}
              </button>
            )}
            <button className="flex items-center gap-2 rounded-lg px-3 py-2 text-left text-body-sm hover:bg-pearl" onClick={() => setShowHistory(true)}>
              <History size={14} strokeWidth={1.75} /> Version history
            </button>
            <button className="rounded-lg px-3 py-2 text-left text-body-sm hover:bg-pearl" onClick={() => setShowTemplate(true)}>Save as template</button>
            {lastRun?.status === "RUNNING" && (
              <button disabled={cancel.isPending} className="rounded-lg px-3 py-2 text-left text-body-sm text-phoenix-orange hover:bg-phoenix-orange/10" onClick={() => cancel.mutate(lastRun.id)}>
                {cancel.isPending ? "Cancelling…" : "Cancel run"}
              </button>
            )}
          </div>
        </details>

        <Button
          className="h-9"
          // GitHub-triggered graphs need the signed payload that only GitHub
          // can provide; a manual start would fail every .trigger reference.
          disabled={nodes.length === 0 || githubTriggered || start.isPending || dirty || saving}
          title={
            githubTriggered
              ? "This workflow runs from a GitHub pull-request webhook"
              : dirty || saving
                ? "Saving…"
                : undefined
          }
          onClick={() =>
            start.mutate(id, { onSuccess: (run) => setWatchingId(run.id) })
          }
        >
          <Play size={13} strokeWidth={2} />
          Run
        </Button>
      </header>

      {(start.error || cancel.error) && (
        <div className="border-b border-hairline px-4 py-2"><ErrorNote>{apiError(start.error ?? cancel.error)}</ErrorNote></div>
      )}

      <RunProvider workspaceId={workspaceId} executionId={runId}>
      <div className="relative flex min-h-0 flex-1">
        <Palette onAdd={addNode} className="hidden lg:flex" />

        <div className="min-w-0 flex-1">
          <FlowCanvas
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={(connection: Connection) =>
              setEdges((current) => addEdge({ ...connection, id: edgeId(current) }, current))
            }
            onAddStep={() => setMobilePanel("steps")}
          />
        </div>

        <Inspector
          node={selected as WorkflowNode | undefined}
          onChange={updateData}
          onDelete={deleteNode}
          className="hidden lg:flex"
        />

        <div className="absolute inset-x-0 bottom-4 z-20 flex justify-center gap-2 px-4 lg:hidden">
          <Button className="h-11 shadow-raised" onClick={() => setMobilePanel("steps")}>
            <Plus size={16} strokeWidth={1.9} /> Add step
          </Button>
          <Button variant="neutral" className="h-11 border border-hairline bg-canvas-white shadow-raised" disabled={!selected} onClick={() => setMobilePanel("inspector")}>
            <SlidersHorizontal size={16} strokeWidth={1.75} /> Configure
          </Button>
        </div>

        {mobilePanel && (
          <>
            <button aria-label="Close panel" className="absolute inset-0 z-30 bg-ink/20 backdrop-blur-[2px] lg:hidden" onClick={() => setMobilePanel(null)} />
            {mobilePanel === "steps" ? (
              <Palette
                onAdd={(type) => {
                  addNode(type);
                  setMobilePanel(null);
                }}
                onClose={() => setMobilePanel(null)}
                className="absolute inset-x-0 bottom-0 z-40 max-h-[78vh] w-auto rounded-t-2xl border-r-0 border-t shadow-featured lg:hidden"
              />
            ) : (
              <Inspector
                node={selected as WorkflowNode | undefined}
                onChange={updateData}
                onDelete={(nodeId) => {
                  deleteNode(nodeId);
                  setMobilePanel(null);
                }}
                onClose={() => setMobilePanel(null)}
                className="absolute inset-x-0 bottom-0 z-40 flex max-h-[78vh] w-auto rounded-t-2xl border-l-0 border-t shadow-featured lg:hidden"
              />
            )}
          </>
        )}
      </div>
      </RunProvider>

      <Modal open={showHistory} onClose={() => setShowHistory(false)} title="Version history">
        <div className="flex flex-col gap-2">
          {versions.isPending && <p className="text-body-sm text-ash">Loading versions…</p>}
          {versions.error && <ErrorNote>{apiError(versions.error)}</ErrorNote>}
          {versions.data?.map((version) => (
            <div key={version.id} className="flex items-center gap-3 rounded-lg border border-hairline p-3">
              <div className="min-w-0 flex-1">
                <p className="text-body-sm text-ink">v{version.number} · {version.name}</p>
                <p className="text-caption text-ash">{new Date(version.createdAt).toLocaleString()}</p>
              </div>
              <Button
                variant="neutral"
                className="h-8 px-3"
                disabled={restore.isPending || version.number === versions.data.length}
                onClick={() => restore.mutate(version.number, { onSuccess: () => setShowHistory(false) })}
              >
                Restore
              </Button>
            </div>
          ))}
        </div>
      </Modal>

      <SaveTemplateModal
        open={showTemplate}
        pending={createTemplate.isPending}
        error={createTemplate.error}
        onClose={() => {
          createTemplate.reset();
          setShowTemplate(false);
        }}
        onSave={(name, description) =>
          createTemplate.mutate(
            { name, description, graph },
            { onSuccess: () => setShowTemplate(false) },
          )
        }
      />
    </div>
  );
}

function SaveTemplateModal({
  open,
  pending,
  error,
  onClose,
  onSave,
}: {
  open: boolean;
  pending: boolean;
  error: unknown;
  onClose: () => void;
  onSave: (name: string, description: string) => void;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  return (
    <Modal open={open} onClose={onClose} title="Save as template">
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          onSave(name, description);
          setName("");
          setDescription("");
        }}
      >
        {error ? <ErrorNote>{apiError(error)}</ErrorNote> : null}
        <label className="flex flex-col gap-1 text-body-sm text-ink">
          Name
          <input
            autoFocus
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            className="rounded-lg border border-hairline bg-canvas-white px-3 py-2 outline-none focus:border-ink"
          />
        </label>
        <label className="flex flex-col gap-1 text-body-sm text-ink">
          Description
          <textarea
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            rows={3}
            className="rounded-lg border border-hairline bg-canvas-white px-3 py-2 outline-none focus:border-ink"
          />
        </label>
        <div className="flex justify-end gap-3">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" disabled={pending}>{pending ? "Saving…" : "Save template"}</Button>
        </div>
      </form>
    </Modal>
  );
}

/**
 * The canvas, with the run drawn over it.
 *
 * Edge state is derived here rather than stored: an edge whose source has
 * finished is one the run has crossed, and marking that on the edges array
 * would mean autosave had a reason to fire every time a node completed.
 */
function FlowCanvas({
  nodes,
  edges,
  onNodesChange,
  onEdgesChange,
  onConnect,
  onAddStep,
}: {
  nodes: Node[];
  edges: Edge[];
  onNodesChange: OnNodesChange<Node>;
  onEdgesChange: OnEdgesChange<Edge>;
  onConnect: (connection: Connection) => void;
  onAddStep: () => void;
}) {
  const { nodes: statuses } = useRunState();

  const drawn = useMemo(
    () =>
      edges.map((edge) => {
        const from = statuses[edge.source];
        const to = statuses[edge.target];
        // Crossed: the source finished and the target is doing something. An
        // edge into a skipped node is the branch that was not taken.
        const taken = from === "SUCCEEDED" && to && to !== "SKIPPED";
        const untaken = to === "SKIPPED";
        return {
          ...edge,
          animated: to === "RUNNING",
          style: {
            stroke: untaken ? "#b1b1af" : taken ? "#8dd087" : undefined,
            strokeWidth: taken ? 2 : undefined,
            strokeDasharray: untaken ? "4 4" : undefined,
            opacity: untaken ? 0.5 : 1,
          },
        };
      }),
    [edges, statuses],
  );

  return (
    <div className="relative size-full">
    <ReactFlow
      nodes={nodes}
      edges={drawn}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      nodeTypes={nodeTypes}
      fitView
      fitViewOptions={{ padding: 0.28, maxZoom: 1 }}
      minZoom={0.35}
      maxZoom={1.5}
      snapToGrid
      snapGrid={[20, 20]}
      zoomOnDoubleClick={false}
      className="bg-cream-wash"
    >
      <Background color="#d4d2cf" gap={20} size={1} />
      <Controls showInteractive={false} />
    </ReactFlow>
      {nodes.length === 0 && (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center p-6">
          <div className="pointer-events-auto flex max-w-xs flex-col items-center rounded-xl border border-hairline bg-canvas-white/95 p-6 text-center shadow-raised backdrop-blur">
            <span className="mb-3 flex size-10 items-center justify-center rounded-xl bg-canary-yellow"><Plus size={18} strokeWidth={1.8} /></span>
            <p className="text-body-lg text-ink">Start with a trigger</p>
            <p className="mt-2 text-caption leading-relaxed text-ash">Add a manual, webhook, schedule, or GitHub trigger. Then connect the steps it should run.</p>
            <Button className="mt-4" onClick={onAddStep}><Plus size={15} /> Add first step</Button>
          </div>
        </div>
      )}
      {nodes.length > 0 && (
        <div className="pointer-events-none absolute left-1/2 top-3 hidden -translate-x-1/2 rounded-full border border-hairline bg-canvas-white/90 px-3 py-1.5 text-caption text-ash shadow-raised backdrop-blur md:block">
          Select a node to configure it · drag a handle to connect
        </div>
      )}
    </div>
  );
}

/**
 * The canvas arrays, reduced to what the API accepts.
 *
 * React Flow decorates its nodes with `measured`, `selected`, `dragging`, and
 * more, and the backend decodes with DisallowUnknownFields — so sending the
 * arrays as-is is a 400, not a stored graph. This is also the right boundary
 * on its own terms: `selected` is a property of this session's canvas, not of
 * the workflow.
 */
function toGraph(nodes: Node[], edges: Edge[]): Graph {
  return {
    nodes: nodes.map((node) => ({
      id: node.id,
      type: node.type ?? "",
      position: { x: Math.round(node.position.x), y: Math.round(node.position.y) },
      data: (node.data ?? {}) as Record<string, unknown>,
    })),
    edges: edges.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      // Omitted rather than sent empty: an edge with no handle is the default
      // path, and the engine tests for exactly that.
      ...(edge.sourceHandle ? { sourceHandle: edge.sourceHandle } : {}),
    })),
  };
}

/**
 * Saves the graph after edits stop.
 *
 * A save is a draft — the backend stores half-built graphs on purpose — so
 * autosaving mid-thought is expected rather than something to guard against.
 * Debounced because dragging a node fires a change per frame.
 */
function useAutosave(workspaceId: string, id: string, graph: Graph) {
  const serialised = JSON.stringify(graph);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  // What the server last accepted, as text. Comparing against it is what stops
  // a re-render with identical content from writing again.
  const savedRef = useRef<string | null>(null);
  const latestRef = useRef(serialised);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const writingRef = useRef(false);
  const flushRef = useRef<() => Promise<void>>(async () => undefined);
  const failedRef = useRef<string | null>(null);

  useEffect(() => {
    latestRef.current = serialised;
  }, [serialised]);

  useEffect(() => {
    flushRef.current = async () => {
      if (writingRef.current || savedRef.current === latestRef.current) return;
      writingRef.current = true;
      const target = latestRef.current;
      setSaving(true);
      try {
        await workflows.update(workspaceId, id, { graph: JSON.parse(target) as Graph });
        savedRef.current = target;
        failedRef.current = null;
        setSaveError(null);
        setDirty(savedRef.current !== latestRef.current);
      } catch (cause) {
        failedRef.current = target;
        setSaveError(apiError(cause));
        setDirty(true);
      } finally {
        writingRef.current = false;
        setSaving(false);
        if (savedRef.current !== latestRef.current && failedRef.current !== latestRef.current) {
          timerRef.current = setTimeout(() => void flushRef.current(), 700);
        }
      }
    };
  }, [id, workspaceId]);

  useEffect(() => {
    if (savedRef.current === null) {
      savedRef.current = serialised;
      return;
    }
    if (savedRef.current === serialised) return;

    if (failedRef.current !== serialised) failedRef.current = null;
    setDirty(true);
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => void flushRef.current(), 700);

    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [serialised]);

  return { saving, saveError, dirty };
}

/** Short, readable, and unique: what a template reference has to be written by hand against. */
function nextId(type: string, existing: Node[]) {
  const stem = (type.split(".")[1] ?? type).slice(0, 8);
  let n = 1;
  while (existing.some((node) => node.id === `${stem}${n}`)) n += 1;
  return `${stem}${n}`;
}

function edgeId(existing: Edge[]) {
  let n = 1;
  while (existing.some((edge) => edge.id === `e${n}`)) n += 1;
  return `e${n}`;
}

function BuilderSkeleton() {
  return (
    <div className="flex h-screen flex-col bg-cream-wash">
      <div className="flex h-[62px] shrink-0 items-center gap-4 border-b border-hairline bg-canvas-white px-4">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="ml-2 h-4 w-40" />
      </div>
      <div className="flex min-h-0 flex-1">
        <div className="w-56 shrink-0 border-r border-hairline bg-canvas-white p-4">
          <Skeleton className="h-40 w-full" />
        </div>
        <div className="flex-1" />
      </div>
    </div>
  );
}
