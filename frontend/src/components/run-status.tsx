import type { ExecutionStatus } from "@/lib/api";

import { cx } from "./ui";

/**
 * The status vocabulary, in one place.
 *
 * These are the engine's own words — the same six the hero animates and the
 * execution rows store — so a run reads identically wherever it is shown. The
 * colour is the data, which is the only thing colour is allowed to be outside
 * the node taxonomy.
 */
const looks: Record<ExecutionStatus, { label: string; dot: string; text: string; live?: boolean }> =
  {
    PENDING: { label: "Queued", dot: "bg-stone", text: "text-ash" },
    RUNNING: { label: "Running", dot: "bg-phoenix-orange", text: "text-ink", live: true },
    SUCCEEDED: { label: "Succeeded", dot: "bg-mint-green", text: "text-ink" },
    FAILED: { label: "Failed", dot: "bg-phoenix-orange", text: "text-phoenix-orange" },
    CANCELLED: { label: "Cancelled", dot: "bg-stone", text: "text-ash" },
    // Only ever a node's, never a run's. It is here so one table covers both.
    SKIPPED: { label: "Skipped", dot: "bg-stone", text: "text-ash" },
  };

export function RunStatus({
  status,
  className,
}: {
  status: ExecutionStatus;
  className?: string;
}) {
  const look = looks[status] ?? looks.PENDING;
  return (
    <span className={cx("inline-flex items-center gap-1.5 text-caption", look.text, className)}>
      <span
        aria-hidden
        // Only a live run pulses. A finished one that kept animating would say
        // something is still happening when nothing is.
        className={cx("size-1.5 shrink-0 rounded-full", look.dot, look.live && "animate-pulse")}
      />
      {look.label}
    </span>
  );
}

/** Server-computed, so every client agrees on what a run took. */
export function formatDuration(ms: number | undefined) {
  if (ms === undefined) return null;
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  const minutes = Math.floor(ms / 60_000);
  return `${minutes}m ${Math.round((ms % 60_000) / 1000)}s`;
}
