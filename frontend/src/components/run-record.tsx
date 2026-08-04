import { categories } from "@/lib/categories";

/**
 * What a finished run looks like afterwards.
 *
 * The claim beside this is that every run keeps its own record — which node ran,
 * how long it took, and what a failure said. A paragraph saying so on an empty
 * half of a dark band is the thing this page kept doing; this shows the record
 * instead. Static on purpose: the hero already carries the motion, and a second
 * animated panel on one page competes with it.
 */
const rows = [
  { node: "trigger", category: "trigger", label: "PR merged", ms: "2ms", state: "ok" },
  { node: "fetch", category: "http", label: "Fetch diff", ms: "412ms", state: "ok" },
  { node: "review", category: "ai", label: "Summarise", ms: "3.1s", state: "ok" },
  { node: "risky", category: "logic", label: "Migrations?", ms: "1ms", state: "ok" },
  { node: "page", category: "http", label: "Page on-call", ms: "—", state: "skipped" },
  {
    node: "post",
    category: "http",
    label: "Post to Slack",
    ms: "284ms",
    state: "failed",
    error: "http POST https://hooks.slack.com/…: 401 Unauthorized",
  },
] as const;

export function RunRecord() {
  return (
    <div className="overflow-hidden rounded-xl bg-[#1b1a19]">
      <div className="flex items-center gap-2 border-b border-white/8 px-4 py-3">
        <span aria-hidden className="size-1.5 rounded-full bg-phoenix-orange" />
        <span className="text-caption text-canvas-white">Run failed</span>
        <span className="ml-auto text-caption text-stone tabular-nums">3.8s</span>
      </div>

      <ul>
        {rows.map((row) => (
          <li key={row.node} className="flex items-center gap-3 px-4 py-2.5">
            <span
              aria-hidden
              className="size-2 shrink-0 rounded-sm"
              style={{
                background: categories[row.category].hex,
                // A skipped node is dimmed rather than hidden. An untaken
                // branch has to look different from a step still pending.
                opacity: row.state === "skipped" ? 0.25 : 1,
              }}
            />
            <span
              className="w-24 shrink-0 truncate text-caption"
              style={{ color: row.state === "skipped" ? "#6d6c6b" : "#ecebea" }}
            >
              {row.label}
            </span>

            {row.state === "failed" ? (
              // The reason lives on the row that failed, which is the whole
              // point of keeping per-node results.
              <span className="min-w-0 flex-1 truncate text-caption text-phoenix-orange">
                {row.error}
              </span>
            ) : (
              <span className="min-w-0 flex-1 truncate text-caption text-stone">
                {row.state === "skipped" ? "Skipped — branch not taken" : "Succeeded"}
              </span>
            )}

            <span className="shrink-0 text-caption text-stone tabular-nums">{row.ms}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
