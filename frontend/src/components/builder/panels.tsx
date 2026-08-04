"use client";

import { Trash2 } from "lucide-react";

import type { WorkflowNode } from "@/lib/api";
import { paletteFor } from "@/lib/categories";
import { nodeSpecs, specFor } from "@/lib/node-types";
import { Button, Eyebrow, Input, Textarea, cx } from "@/components/ui";

/** The list of node types you can add, grouped nowhere — there are seven. */
export function Palette({ onAdd }: { onAdd: (type: string) => void }) {
  return (
    <aside className="flex w-56 shrink-0 flex-col gap-2 overflow-y-auto border-r border-hairline bg-canvas-white p-4">
      <Eyebrow className="mb-1">Add a node</Eyebrow>
      {nodeSpecs.map((spec) => {
        const palette = paletteFor(spec.type);
        return (
          <button
            key={spec.type}
            onClick={() => onAdd(spec.type)}
            className="flex items-center gap-2.5 rounded-lg border border-hairline px-3 py-2 text-left hover:bg-pearl"
          >
            <span
              aria-hidden
              className="size-3 shrink-0 rounded-sm"
              style={{ background: palette.hex }}
            />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-body-sm text-ink">{spec.label}</span>
              <span className="block truncate text-[10px] text-ash">{spec.type}</span>
            </span>
          </button>
        );
      })}
    </aside>
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
}: {
  node: WorkflowNode | undefined;
  onChange: (id: string, data: Record<string, unknown>) => void;
  onDelete: (id: string) => void;
}) {
  if (!node) {
    return (
      <aside className="hidden w-80 shrink-0 border-l border-hairline bg-canvas-white p-4 lg:block">
        <Eyebrow>Inspector</Eyebrow>
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
    <aside className="flex w-80 shrink-0 flex-col gap-5 overflow-y-auto border-l border-hairline bg-canvas-white p-4">
      <div className="flex items-start justify-between gap-2">
        <div className="flex flex-col gap-1">
          <Eyebrow>{paletteFor(node.type).label}</Eyebrow>
          <span className="text-body-sm text-ink">{spec?.label ?? node.type}</span>
          <span className="text-[10px] text-stone">{node.id}</span>
        </div>
        <button
          onClick={() => onDelete(node.id)}
          aria-label="Delete node"
          className="-m-1 rounded-lg p-1 text-stone hover:text-phoenix-orange"
        >
          <Trash2 size={16} strokeWidth={1.75} />
        </button>
      </div>

      <label className="flex flex-col gap-2">
        <Eyebrow>Label</Eyebrow>
        <Input
          value={typeof data.label === "string" ? data.label : ""}
          onChange={(event) => set("label", event.target.value)}
          placeholder={spec?.label ?? "Name this step"}
        />
      </label>

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
