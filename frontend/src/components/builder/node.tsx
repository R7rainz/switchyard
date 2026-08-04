"use client";

import { Handle, Position, type NodeProps } from "@xyflow/react";

import { paletteFor } from "@/lib/categories";
import { conditionHandles, specFor } from "@/lib/node-types";
import { cx } from "@/components/ui";

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

  return (
    <div
      className={cx(
        "w-[200px] overflow-hidden rounded-xl border bg-canvas-white",
        selected ? "border-ink" : "border-hairline",
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

      <div className="h-1.5" style={{ background: palette.hex }} />

      <div className="flex flex-col gap-1 px-3 py-2.5">
        <span className="text-eyebrow uppercase tracking-[0.3px] text-ash">{palette.label}</span>
        <span className="truncate text-body-sm text-ink">{label}</span>
        {/* The id, because every template reference is written against it. */}
        <span className="truncate text-[10px] text-stone">{id}</span>
      </div>

      {type === "logic.condition" ? (
        // Two named outputs. The handle id becomes the edge's sourceHandle, and
        // the engine follows an edge only when that name matches the branch the
        // node returned — so these strings are a contract, not decoration.
        conditionHandles.map((branch, index) => (
          <Handle
            key={branch}
            id={branch}
            type="source"
            position={Position.Right}
            style={{ top: `${38 + index * 26}%` }}
            className={cx(
              "!size-2.5 !border-2 !border-canvas-white",
              branch === "true" ? "!bg-mint-green" : "!bg-phoenix-orange",
            )}
          />
        ))
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
    "logic.condition",
    "variable.set",
    "http.request",
    "ai.prompt",
  ].map((type) => [type, BuilderNode]),
);
