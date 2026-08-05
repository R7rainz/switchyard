"use client";

import Link from "next/link";
import { ChevronRight } from "lucide-react";

import { PageHeader } from "@/components/app-shell";
import { RunStatus, formatDuration } from "@/components/run-status";
import { EmptyState, ErrorNote, Eyebrow, Skeleton, cx } from "@/components/ui";
import { apiError } from "@/lib/api";
import { relativeTime } from "@/lib/time";
import { useExecutions, useWorkflows, useWorkspace } from "@/lib/queries";

/**
 * Every run this workspace has had.
 *
 * The dashboard's strip shows the last handful and the builder shows the one
 * you just started; neither answers "what has this workspace been doing", which
 * is the question an audit trail exists for. Explainability is a principle here,
 * and a record nothing links to is a record nobody reads.
 */
export default function RunsPage() {
  const { workspace } = useWorkspace();
  const { data: runs, isPending, error } = useExecutions(workspace?.id, 100);
  const { data: flows } = useWorkflows(workspace?.id);

  const nameOf = (workflowId: string | undefined) =>
    // A run outlives the workflow it ran — the execution keeps its own graph
    // snapshot on purpose, and the foreign key is set null on delete.
    flows?.find((flow) => flow.id === workflowId)?.name ?? "Deleted workflow";

  return (
    <>
      <PageHeader eyebrow="Workspace" title="Runs" />

      {error && <ErrorNote>{apiError(error)}</ErrorNote>}
      {isPending && <RunsSkeleton />}

      {runs && runs.length === 0 && (
        <EmptyState
          title="Nothing has run yet"
          hint="Open a workflow and press Run. Every execution is recorded here with what each node did and how long it took."
        />
      )}

      {runs && runs.length > 0 && (
        <ul className="overflow-hidden rounded-xl border border-hairline bg-canvas-white">
          {runs.map((run, index) => (
            <li key={run.id} className={cx(index > 0 && "border-t border-hairline")}>
              <Link
                href={`/runs/${run.id}`}
                className="flex flex-wrap items-center gap-x-4 gap-y-1 px-5 py-3.5 hover:bg-pearl"
              >
                <RunStatus status={run.status} className="w-24 shrink-0" />

                <span className="min-w-0 flex-1 truncate text-body-sm text-ink">
                  {nameOf(run.workflowId)}
                </span>

                {run.error && (
                  // The reason, not just the fact. A failed run whose message is
                  // one click away is a failed run nobody reads.
                  <span className="min-w-0 basis-full truncate text-caption text-ash sm:max-w-xs sm:basis-auto">
                    {run.error}
                  </span>
                )}

                <span className="shrink-0 text-caption text-ash tabular-nums">
                  {formatDuration(run.durationMs) ?? "—"}
                </span>
                {/* Wide enough for "44 seconds ago" at the eyebrow's letter
                    spacing — w-24 wrapped it onto two lines. */}
                <Eyebrow className="w-28 shrink-0 text-right">
                  {relativeTime(run.createdAt)}
                </Eyebrow>
                <ChevronRight size={14} strokeWidth={1.75} className="shrink-0 text-stone" />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

function RunsSkeleton() {
  return (
    <div className="rounded-xl border border-hairline bg-canvas-white">
      {[0, 1, 2, 3, 4].map((row) => (
        <div key={row} className="flex items-center gap-4 px-5 py-3.5">
          <Skeleton className="h-3 w-20" />
          <Skeleton className="h-3 flex-1" />
          <Skeleton className="h-3 w-16" />
        </div>
      ))}
    </div>
  );
}
