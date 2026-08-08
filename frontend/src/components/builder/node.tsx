"use client";

import { Handle, Position, type NodeProps } from "@xyflow/react";
import { createElement } from "react";

import { iconFor, paletteFor } from "@/lib/categories";
import { conditionHandles, specFor, switchHandles } from "@/lib/node-types";
import { cx } from "@/components/ui";
import { useNodeStatus } from "./run-state";

/**
 * Every node type renders through this one component.
 *
 * React Flow picks a component by `node.type`, and `node.type` is also the
 * value the engine dispatches on — registering one component for all of them
 * keeps the field the backend reads out of the frontend's rendering concerns.
 */
export function BuilderNode({ id, type, data, selected }: NodeProps) {
  const spec = specFor(type);
  const palette = paletteFor(type);
  const label = typeof data?.label === "string" && data.label ? data.label : (spec?.label ?? type);
  const isTrigger = type.startsWith("trigger.");
  const branches = type === "logic.switch"
    ? switchHandles(data as Record<string, unknown>)
    : type === "logic.condition" || type === "ai.decision"
      ? [...conditionHandles]
      : null;

  // What this node is doing in the run being watched, if any. It comes from a
  // context rather than from node.data, because data is what autosave writes —
  // a status stored there would become part of the workflow's definition.
  const run = useNodeStatus(id);

  return (
    <div
      style={{ minHeight: branches ? Math.max(84, branches.length * 24 + 28) : undefined }}
      className={cx(
        "relative w-[216px] overflow-hidden rounded-xl border bg-canvas-white shadow-[0_1px_2px_rgba(17,17,17,0.03)] transition-[border-color,opacity,box-shadow,transform] duration-200 hover:-translate-y-0.5 hover:shadow-raised",
        run === "RUNNING" && "border-phoenix-orange shadow-[0_0_0_3px_rgba(232,64,13,0.15)]",
        run === "SUCCEEDED" && "border-mint-green",
        run === "FAILED" && "border-phoenix-orange",
        // Dimmed, not hidden. An untaken branch has to look different from a
        // step that is still waiting — that distinction is why the engine
        // records SKIPPED at all.
        run === "SKIPPED" && "opacity-40",
        !run && (selected ? "border-ink shadow-raised" : "border-hairline"),
        run && selected && "ring-1 ring-ink",
      )}
    >
      {/* A trigger has nothing before it — the engine rejects an edge into one,
          so there is no handle to draw. */}
      {!isTrigger && (
        <Handle
          type="target"
          position={Position.Left}
          className="!size-2.5 !border-2 !border-canvas-white !bg-stone"
        />
      )}

      <div className="h-1" style={{ background: palette.hex }} />

      <div className="flex flex-col gap-1 px-3 py-2.5">
        <span className="flex items-center gap-1.5">
          <span className="flex size-6 items-center justify-center rounded-md" style={{ background: `${palette.hex}99` }}>
            {createElement(iconFor(type), { size: 13, strokeWidth: 1.8, className: "text-ink" })}
          </span>
          <span className="text-eyebrow uppercase tracking-[0.3px] text-ash">{palette.label}</span>
          {run && (
            <span
              aria-hidden
              className={cx(
                "size-1.5 rounded-full",
                run === "RUNNING" && "animate-pulse bg-phoenix-orange",
                run === "SUCCEEDED" && "bg-mint-green",
                run === "FAILED" && "bg-phoenix-orange",
                (run === "SKIPPED" || run === "PENDING" || run === "CANCELLED") && "bg-stone",
              )}
            />
          )}
        </span>
        <span className="truncate text-body-sm text-ink">{label}</span>
        {/* The id, because every template reference is written against it. */}
        <span className="truncate text-[10px] text-stone">{id}</span>
      </div>

      {branches ? (
        // Named outputs. The handle id becomes the edge's sourceHandle, and
        // the engine follows an edge only when that name matches the branch the
        // node returned — so these strings are a contract, not decoration.
        branches.map((branch, index) => {
          const top = `${((index + 1) / (branches.length + 1)) * 100}%`;
          return (
            <div key={branch}>
              <span className="pointer-events-none absolute right-2 -translate-y-1/2 rounded bg-canvas-white px-1 text-[9px] text-ash" style={{ top }}>{branch}</span>
              <Handle
                id={branch}
                type="source"
                position={Position.Right}
                style={{ top }}
                className={cx(
                  "!size-2.5 !border-2 !border-canvas-white",
                  branch === "false" || branch === "default" ? "!bg-phoenix-orange" : "!bg-mint-green",
                )}
              />
            </div>
          );
        })
      ) : (
        <Handle
          type="source"
          position={Position.Right}
          className="!size-2.5 !border-2 !border-canvas-white !bg-stone"
        />
      )}
    </div>
  );
}

/**
 * Built once at module scope. React Flow warns and remounts every node if this
 * object is recreated on each render.
 */
export const nodeTypes = Object.fromEntries(
  [
    "trigger.manual",
    "trigger.webhook",
    "trigger.schedule",
    "trigger.github.pull_request",
    "logic.condition",
    "logic.switch",
    "logic.delay",
    "variable.set",
    "http.request",
    "ai.prompt",
    "ai.chat",
    "ai.summarize",
    "ai.classification",
    "ai.decision",
    "github.pull_request",
    "github.issue",
    "github.comment",
    "github.merge",
    "slack.message",
    "discord.message",
    "email.message",
  ].map((type) => [type, BuilderNode]),
);
