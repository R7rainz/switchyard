"use client";

import { Eyebrow, Skeleton, cx } from "./ui";
import { RunStatus, formatDuration } from "./run-status";
import { useExecutions, useWorkflows } from "@/lib/queries";
import { relativeTime } from "@/lib/time";

/**
 * What this workspace has actually been doing.
 *
 * The dashboard was a list of names with no sense of anything ever having
 * happened, while the backend had every run recorded and none of it reachable.
 * This is that data, at a glance: what ran, how it went, how long it took.
 *
 * It is absent rather than empty when nothing has run — an empty panel headed
 * "Recent runs" on a brand new workspace is a promise the screen cannot keep
 * yet, and the workflow grid already carries its own empty state.
 */
export function RecentRuns({ workspaceId }: { workspaceId: string | undefined }) {
  const { data: runs, isPending } = useExecutions(workspaceId);
  const { data: flows } = useWorkflows(workspaceId);

  if (isPending) return <RecentRunsSkeleton />;
  if (!runs || runs.length === 0) return null;

  const nameOf = (workflowId: string | undefined) =>
    flows?.find((flow) => flow.id === workflowId)?.name ??
    // A run outlives the workflow it ran: the execution keeps its own graph
    // snapshot on purpose, and the foreign key is set null on delete.
    "Deleted workflow";

  return (
    <section className="mt-12">
      <div className="mb-4 flex items-end justify-between gap-4">
        <Eyebrow>Recent runs</Eyebrow>
        <span className="text-caption text-ash">Updates automatically</span>
      </div>

      <ul className="overflow-hidden rounded-xl border border-hairline bg-canvas-white">
        {runs.map((run, index) => (
          <li
            key={run.id}
            className={cx(
              "flex flex-wrap items-center gap-x-4 gap-y-1 px-5 py-3.5",
              index > 0 && "border-t border-hairline",
            )}
          >
            <RunStatus status={run.status} className="w-24 shrink-0" />

            <span className="min-w-0 flex-1 truncate text-body-sm text-ink">
              {nameOf(run.workflowId)}
            </span>

            {run.error && (
              // The reason, not just the fact. A failed run whose message is one
              // click away is a failed run nobody reads.
              <span className="min-w-0 basis-full truncate text-caption text-ash sm:basis-auto sm:max-w-xs">
                {run.error}
              </span>
            )}

            <span className="shrink-0 text-caption text-ash tabular-nums">
              {formatDuration(run.durationMs) ?? "—"}
            </span>
            <Eyebrow className="w-24 shrink-0 text-right">{relativeTime(run.createdAt)}</Eyebrow>
          </li>
        ))}
      </ul>
    </section>
  );
}

function RecentRunsSkeleton() {
  return (
    <section className="mt-12">
      <Skeleton className="mb-4 h-3 w-24" />
      <div className="rounded-xl border border-hairline bg-canvas-white">
        {[0, 1, 2].map((row) => (
          <div key={row} className="flex items-center gap-4 px-5 py-3.5">
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-3 flex-1" />
            <Skeleton className="h-3 w-16" />
          </div>
        ))}
      </div>
    </section>
  );
}
