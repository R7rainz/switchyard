"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { ArrowLeft, ChevronDown } from "lucide-react";

import { RunProvider, useRunState } from "@/components/builder/run-state";
import { GraphPreview } from "@/components/graph-preview";
import { RunStatus, formatDuration } from "@/components/run-status";
import { Button, Eyebrow, ErrorNote, Skeleton, cx } from "@/components/ui";
import type { ExecutionStatus, WorkflowNode } from "@/lib/api";
import { paletteFor } from "@/lib/categories";
import { specFor } from "@/lib/node-types";
import { relativeTime } from "@/lib/time";
import { useCancelExecution, useWorkflows, useWorkspace } from "@/lib/queries";

/**
 * One run, in full.
 *
 * Everything here comes from RunProvider, which is the same thing the builder
 * canvas watches with — one socket, one snapshot, one set of merge rules. A
 * second fetching path for the same data is how two screens come to disagree
 * about what a run did.
 */
export default function RunPage() {
  const { id } = useParams<{ id: string }>();
  const { workspace } = useWorkspace();

  if (!workspace) return <RunSkeleton />;

  return (
    <RunProvider workspaceId={workspace.id} executionId={id}>
      <RunView workspaceId={workspace.id} />
    </RunProvider>
  );
}

function RunView({ workspaceId }: { workspaceId: string }) {
  const run = useRunState();
  const { data: flows } = useWorkflows(workspaceId);
  const cancel = useCancelExecution(workspaceId);

  if (run.loadError) return <ErrorNote>{run.loadError}</ErrorNote>;

  // The row has not landed yet. Status may already be set from an event, but
  // there is nothing to lay out until the graph arrives.
  if (!run.execution || !run.graph) return <RunSkeleton />;

  const record = run.execution;
  const workflow = flows?.find((flow) => flow.id === record.workflowId);
  const status = run.status ?? record.status;

  return (
    <>
      <div className="mb-8 flex flex-wrap items-start justify-between gap-4">
        <div className="flex min-w-0 flex-col gap-3">
          <Link href="/runs" className="flex items-center gap-2 text-ash hover:text-ink">
            <ArrowLeft size={14} strokeWidth={1.75} />
            <Eyebrow>All runs</Eyebrow>
          </Link>
          <h1 className="truncate text-heading-sm text-ink">
            {workflow?.name ?? "Deleted workflow"}
          </h1>
        </div>

        <div className="flex items-center gap-3">
          {/* Cancel reaches only runs this process is executing. One that
              finished a moment ago answers 409, which is honest rather than an
              error worth showing. */}
          {status === "RUNNING" && (
            <Button
              variant="neutral"
              className="h-9"
              disabled={cancel.isPending}
              onClick={() => cancel.mutate(record.id)}
            >
              {cancel.isPending ? "Cancelling…" : "Cancel"}
            </Button>
          )}
          {workflow && (
            <Link
              href={`/workflows/${workflow.id}`}
              className="inline-flex h-9 items-center rounded-lg bg-pearl px-4 text-body-sm text-ink hover:bg-stone/40"
            >
              Open in builder
            </Link>
          )}
        </div>
      </div>

      <div className="grid items-center gap-4 rounded-xl border border-hairline bg-canvas-white p-5 sm:grid-cols-[1fr_auto]">
        <div className="grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-4">
          <Fact label="Status">
            <RunStatus status={status} />
          </Fact>
          <Fact label="Duration">{formatDuration(record.durationMs) ?? "—"}</Fact>
          <Fact label="Trigger">{record.trigger}</Fact>
          <Fact label="Started">{relativeTime(record.startedAt ?? record.createdAt)}</Fact>
        </div>

        {/* The graph as it was when this ran, which is not necessarily what the
            workflow says today — that is the point of storing the snapshot. */}
        <GraphPreview graph={run.graph} className="h-24 w-60 shrink-0" />
      </div>

      {run.error && (
        <div className="mt-4">
          <ErrorNote>{run.error}</ErrorNote>
        </div>
      )}

      <Eyebrow className="mb-4 mt-10 block">Nodes</Eyebrow>

      <ul className="overflow-hidden rounded-xl border border-hairline bg-canvas-white">
        {run.graph.nodes.map((node, index) => (
          <li key={node.id} className={cx(index > 0 && "border-t border-hairline")}>
            <NodeRow node={node} />
          </li>
        ))}
      </ul>
    </>
  );
}

/**
 * What one node did.
 *
 * A node with no record has not been reached — which is a different thing from
 * SKIPPED, and the engine keeps them different on purpose: skipped is a branch
 * that was decided against, and nothing here is a step the run never got to.
 */
function NodeRow({ node }: { node: WorkflowNode }) {
  const run = useRunState();
  const status: ExecutionStatus | undefined = run.nodes[node.id];
  const output = run.nodeOutputs[node.id];
  const error = run.nodeErrors[node.id];
  const timing = run.nodeTimes[node.id];

  const palette = paletteFor(node.type);
  const label =
    (typeof node.data?.label === "string" && node.data.label) ||
    specFor(node.type)?.label ||
    node.type;

  const body = (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 py-3.5 pl-5 pr-3">
      <span
        aria-hidden
        className="size-2.5 shrink-0 rounded-sm"
        // A skipped node is dimmed rather than hidden. An untaken branch has to
        // look different from a step that never ran.
        style={{ background: palette.hex, opacity: status === "SKIPPED" ? 0.3 : 1 }}
      />

      <span className="w-40 shrink-0 truncate text-body-sm text-ink">{label}</span>
      {/* The id, because every template reference is written against it. */}
      <span className="w-20 shrink-0 truncate text-[10px] text-stone">{node.id}</span>

      <span className="min-w-0 flex-1 truncate text-caption text-phoenix-orange">{error}</span>

      {status ? (
        <RunStatus status={status} className="w-24 shrink-0 justify-end" />
      ) : (
        <span className="w-24 shrink-0 text-right text-caption text-stone">Not reached</span>
      )}
      <span className="w-16 shrink-0 text-right text-caption text-ash tabular-nums">
        {formatDuration(timing?.durationMs) ?? "—"}
      </span>
      <ChevronDown
        size={14}
        strokeWidth={1.75}
        aria-hidden
        className={cx(
          "shrink-0 text-stone transition-transform group-open:rotate-180",
          // Held in the layout rather than removed, so a row with an output and
          // a row without keep their columns lined up.
          output === undefined && "invisible",
        )}
      />
    </div>
  );

  // Nothing to expand into. A disclosure arrow that opens on an empty panel is
  // worse than no arrow.
  if (output === undefined) return body;

  return (
    <details className="group">
      <summary className="cursor-pointer list-none hover:bg-pearl [&::-webkit-details-marker]:hidden">
        {body}
      </summary>
      {/* A failed node keeps its output deliberately — an HTTP node rejected
          with a 401 still holds the body saying why, and that body is the whole
          reason anyone opens a failed run. */}
      <pre className="max-h-96 overflow-auto border-t border-hairline bg-cream-wash px-5 py-4 font-mono text-[11px] leading-relaxed text-ink">
        {JSON.stringify(output, null, 2)}
      </pre>
    </details>
  );
}

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Eyebrow>{label}</Eyebrow>
      <span className="truncate text-body-sm text-ink">{children}</span>
    </div>
  );
}

function RunSkeleton() {
  return (
    <>
      <Skeleton className="mb-8 h-10 w-64" />
      <Skeleton className="h-28 w-full rounded-xl" />
      <Skeleton className="mt-10 h-64 w-full rounded-xl" />
    </>
  );
}
