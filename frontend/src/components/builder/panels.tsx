"use client";

import { ChevronDown, Search, Trash2, X } from "lucide-react";
import { createElement, useMemo, useState } from "react";

import type { WorkflowNode } from "@/lib/api";
import { iconFor, paletteFor } from "@/lib/categories";
import { nodeSpecs, specFor } from "@/lib/node-types";
import { Button, Eyebrow, Input, Textarea, cx } from "@/components/ui";
import { RunStatus } from "@/components/run-status";
import { useNodeResult } from "./run-state";

/** The searchable node library, grouped so the full catalog never lands at once. */
export function Palette({
  onAdd,
  onClose,
  className,
}: {
  onAdd: (type: string) => void;
  onClose?: () => void;
  className?: string;
}) {
  const [search, setSearch] = useState("");
  const filtered = useMemo(
    () => nodeSpecs.filter((spec) => `${spec.label} ${spec.summary} ${spec.type}`.toLowerCase().includes(search.toLowerCase())),
    [search],
  );
  const groups = [...new Set(filtered.map((spec) => paletteFor(spec.type).label))];
  return (
    <aside className={cx("flex w-72 shrink-0 flex-col gap-3 overflow-y-auto border-r border-hairline bg-canvas-white p-4", className)}>
      <div className="flex items-center justify-between gap-3">
        <div>
          <Eyebrow>Step library</Eyebrow>
          <p className="mt-1 text-caption text-ash">Choose what happens next.</p>
        </div>
        {onClose ? (
          <button onClick={onClose} aria-label="Close step library" className="rounded-lg p-2 text-ash hover:bg-pearl hover:text-ink">
            <X size={16} strokeWidth={1.75} />
          </button>
        ) : <span className="text-[10px] text-stone">{filtered.length}</span>}
      </div>
      <label className="relative">
        <Search size={14} className="pointer-events-none absolute left-3 top-3 text-stone" />
        <input aria-label="Search node types" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search nodes" className="h-10 w-full rounded-lg border border-hairline bg-cream-wash pl-9 pr-3 text-body-sm text-ink outline-none placeholder:text-stone focus:border-ink/25" />
      </label>
      {search ? filtered.map((spec) => <PaletteItem key={spec.type} spec={spec} onAdd={onAdd} />) : groups.map((group, index) => (
        <details key={group} open={index < 2} className="group rounded-xl border border-hairline bg-canvas-white">
          <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2.5">
            <span className="flex-1 text-body-sm text-ink">{group}</span>
            <span className="text-caption text-stone">{filtered.filter((spec) => paletteFor(spec.type).label === group).length}</span>
            <ChevronDown size={14} strokeWidth={1.75} className="text-stone transition-transform group-open:rotate-180" />
          </summary>
          <div className="flex flex-col gap-1 border-t border-hairline p-1.5">
            {filtered.filter((spec) => paletteFor(spec.type).label === group).map((spec) => (
              <PaletteItem key={spec.type} spec={spec} onAdd={onAdd} />
            ))}
          </div>
        </details>
      ))}
      {filtered.length === 0 && <p className="px-1 py-3 text-caption text-ash">No nodes match that search.</p>}
    </aside>
  );
}

function PaletteItem({ spec, onAdd }: { spec: (typeof nodeSpecs)[number]; onAdd: (type: string) => void }) {
  const palette = paletteFor(spec.type);
  return (
    <button onClick={() => onAdd(spec.type)} title={spec.summary} className="flex items-center gap-2.5 rounded-lg border border-transparent px-2.5 py-2 text-left hover:border-hairline hover:bg-cream-wash">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-lg" style={{ background: `${palette.hex}99` }}>
        {createElement(iconFor(spec.type), { size: 15, strokeWidth: 1.8 })}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-body-sm text-ink">{spec.label}</span>
        <span className="block truncate text-[10px] text-ash">{spec.summary}</span>
      </span>
    </button>
  );
}

/**
 * The selected node's configuration.
 *
 * Fields come from the node's spec, and everything is written straight into
 * `data` — which the backend keeps as opaque JSON. That is what lets a field
 * be added here without a migration, and why an unrecognised key survives a
 * save and load unchanged.
 */
export function Inspector({
  node,
  onChange,
  onDelete,
  onClose,
  className,
}: {
  node: WorkflowNode | undefined;
  onChange: (id: string, data: Record<string, unknown>) => void;
  onDelete: (id: string) => void;
  onClose?: () => void;
  className?: string;
}) {
  if (!node) {
    return (
      <aside className={cx("hidden w-80 shrink-0 border-l border-hairline bg-canvas-white p-4 lg:block", className)}>
        <div className="flex items-center justify-between gap-3">
          <Eyebrow>Inspector</Eyebrow>
          {onClose && <button onClick={onClose} aria-label="Close inspector" className="rounded-lg p-2 text-ash hover:bg-pearl hover:text-ink"><X size={16} strokeWidth={1.75} /></button>}
        </div>
        <p className="mt-4 text-body-sm leading-relaxed text-ash">
          Select a node to configure it. Drag from a node&apos;s right edge to connect it to the
          next one.
        </p>
      </aside>
    );
  }

  const spec = specFor(node.type);
  const data = node.data ?? {};
  const set = (key: string, value: unknown) => onChange(node.id, { ...data, [key]: value });

  return (
    <aside className={cx("flex w-80 shrink-0 flex-col gap-5 overflow-y-auto border-l border-hairline bg-canvas-white p-4", className)}>
      <div className="flex items-start justify-between gap-2">
        <div className="flex flex-col gap-1">
          <Eyebrow>{paletteFor(node.type).label}</Eyebrow>
          <span className="text-body-sm text-ink">{spec?.label ?? node.type}</span>
          <span className="text-[10px] text-stone">{node.id}</span>
        </div>
        <div className="flex items-center gap-1">
          <button onClick={() => onDelete(node.id)} aria-label="Delete node" className="rounded-lg p-2 text-stone hover:bg-phoenix-orange/10 hover:text-phoenix-orange">
            <Trash2 size={16} strokeWidth={1.75} />
          </button>
          {onClose && <button onClick={onClose} aria-label="Close inspector" className="rounded-lg p-2 text-ash hover:bg-pearl hover:text-ink"><X size={16} strokeWidth={1.75} /></button>}
        </div>
      </div>

      {spec?.summary && <p className="rounded-lg bg-cream-wash p-3 text-caption leading-relaxed text-ash">{spec.summary}</p>}

      <NodeResult nodeId={node.id} />

      <label className="flex flex-col gap-2">
        <Eyebrow>Label</Eyebrow>
        <Input
          value={typeof data.label === "string" ? data.label : ""}
          onChange={(event) => set("label", event.target.value)}
          placeholder={spec?.label ?? "Name this step"}
        />
      </label>

      {spec?.fields.length ? <Eyebrow>Configuration</Eyebrow> : null}
      {spec?.fields.map((field) => (
        <label key={field.key} className="flex flex-col gap-2">
          <Eyebrow>{field.label}</Eyebrow>

          {field.kind === "select" ? (
            <select
              value={typeof data[field.key] === "string" ? (data[field.key] as string) : ""}
              onChange={(event) => set(field.key, event.target.value)}
              className="h-12 rounded-xl border border-hairline bg-canvas-white px-4 text-body-sm text-ink focus:border-ink/25 focus:outline-none"
            >
              <option value="">Default</option>
              {field.options?.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </select>
          ) : field.kind === "text" ? (
            <Input
              value={typeof data[field.key] === "string" ? (data[field.key] as string) : ""}
              onChange={(event) => set(field.key, event.target.value)}
              placeholder={field.placeholder}
            />
          ) : (
            // JSON and long text share a textarea. JSON is stored as the string
            // the user typed rather than parsed on every keystroke — half-typed
            // JSON is not valid JSON, and refusing to store it would mean the
            // field fights back while it is being filled in.
            <Textarea
              rows={field.kind === "json" ? 5 : 4}
              value={valueAsText(data[field.key])}
              onChange={(event) => set(field.key, event.target.value)}
              placeholder={field.placeholder}
              className={cx(field.kind === "json" && "font-mono text-[12px]")}
            />
          )}

          {field.hint && (
            <span className="text-[11px] leading-relaxed text-ash">{field.hint}</span>
          )}
        </label>
      ))}
    </aside>
  );
}

function valueAsText(value: unknown) {
  if (value === undefined || value === null) return "";
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2);
}

/** The save indicator. A builder that autosaves has to say when it has. */
export function SaveState({
  saving,
  error,
  dirty,
}: {
  saving: boolean;
  error: string | null;
  dirty: boolean;
}) {
  if (error) return <span className="text-caption text-phoenix-orange">{error}</span>;
  if (saving) return <span className="text-caption text-ash">Saving…</span>;
  if (dirty) return <span className="text-caption text-ash">Unsaved</span>;
  return <span className="text-caption text-ash">Saved</span>;
}

export { Button };

/**
 * What this node did in the run being watched.
 *
 * The engine keeps a failed node's output deliberately — an HTTP node rejected
 * with a 401 still holds the body saying why, and that body is the reason
 * anyone opens a failed run. Showing the config without it means the answer is
 * on the server and nowhere a person can see it.
 */
function NodeResult({ nodeId }: { nodeId: string }) {
  const { status, output, error } = useNodeResult(nodeId);
  if (!status) return null;

  return (
    <div className="flex flex-col gap-2 rounded-xl bg-cream-wash p-3">
      <div className="flex items-center justify-between gap-2">
        <Eyebrow>Last run</Eyebrow>
        <RunStatus status={status} />
      </div>

      {error && <p className="text-caption leading-relaxed text-phoenix-orange">{error}</p>}

      {output !== undefined && (
        <pre className="max-h-48 overflow-auto rounded-lg bg-canvas-white p-2 font-mono text-[11px] leading-relaxed text-ink">
          {JSON.stringify(output, null, 2)}
        </pre>
      )}
    </div>
  );
}
