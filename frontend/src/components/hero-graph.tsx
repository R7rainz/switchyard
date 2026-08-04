"use client";

import { useEffect, useState } from "react";

import { categories } from "@/lib/categories";

/**
 * A workflow running, in the frame DESIGN.md asks the product to be shown in.
 *
 * The document's imagery guidance is "UI-in-UI: showing the product is the
 * imagery" — so rather than describe branching and live execution in prose, the
 * hero runs a workflow: nodes go from pending to running to done in dependency
 * order, one branch is taken and the other is visibly skipped, and it starts
 * over. Everything it shows is behaviour the engine actually has.
 */

type Step = {
  id: string;
  label: string;
  category: keyof typeof categories;
  x: number;
  y: number;
  /** Which branch of the condition this node sits on, if any. */
  branch?: "true" | "false";
};

const steps: Step[] = [
  { id: "trigger", label: "PR merged", category: "trigger", x: 0, y: 1 },
  { id: "fetch", label: "Fetch diff", category: "http", x: 1, y: 1 },
  { id: "review", label: "Summarise", category: "ai", x: 2, y: 1 },
  { id: "risky", label: "Migrations?", category: "logic", x: 3, y: 1 },
  { id: "page", label: "Page on-call", category: "http", x: 4, y: 0, branch: "true" },
  { id: "post", label: "Post to Slack", category: "http", x: 4, y: 2, branch: "false" },
];

const edges: Array<{ from: string; to: string; branch?: "true" | "false" }> = [
  { from: "trigger", to: "fetch" },
  { from: "fetch", to: "review" },
  { from: "review", to: "risky" },
  { from: "risky", to: "page", branch: "true" },
  { from: "risky", to: "post", branch: "false" },
];

/** The branch this run takes. The other one is recorded SKIPPED, as it is in the engine. */
const TAKEN = "false";

const order = ["trigger", "fetch", "review", "risky", "page", "post"];
const STEP_MS = 900;

/*
 * One set of numbers for both layers. The edges are an SVG and the nodes are
 * absolutely positioned divs, so anything measured twice is a chance for the
 * line to miss the box it points at.
 */
const COL = 128;
const ROW = 78;
const NODE_W = 116;
const NODE_H = 52;
const WIDTH = COL * 4 + NODE_W;
const HEIGHT = ROW * 2 + NODE_H;

export function HeroGraph() {
  // -1 is "nothing has started"; past the end is the pause before the loop.
  const [cursor, setCursor] = useState(-1);

  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      // Reduced motion gets the finished run rather than no run: the picture is
      // the point, the sequencing is the flourish.
      //
      // Deferred rather than set inline. The initial value has to match what
      // the server rendered, so it cannot be read from matchMedia up front —
      // and setting state synchronously inside an effect cascades a second
      // render before the first has painted.
      const settle = setTimeout(() => setCursor(order.length), 0);
      return () => clearTimeout(settle);
    }
    const timer = setInterval(
      () => setCursor((at) => (at > order.length + 1 ? -1 : at + 1)),
      STEP_MS,
    );
    return () => clearInterval(timer);
  }, []);

  const statusOf = (id: string) => {
    const step = steps.find((one) => one.id === id)!;
    if (step.branch && step.branch !== TAKEN) {
      // The untaken branch is skipped, not pending — that distinction is the
      // whole reason the engine records SKIPPED, so the picture keeps it.
      return cursor >= order.indexOf(id) ? "skipped" : "pending";
    }
    const index = order.indexOf(id);
    if (cursor > index) return "done";
    if (cursor === index) return "running";
    return "pending";
  };

  return (
    <div className="rounded-xl bg-charcoal p-2 shadow-[var(--shadow-featured)]">
      {/* Window chrome, so the panel reads as a capture of the product. */}
      <div className="flex items-center gap-2 px-3 py-2">
        <span className="flex gap-1.5" aria-hidden>
          <span className="size-2.5 rounded-full bg-canvas-white/20" />
          <span className="size-2.5 rounded-full bg-canvas-white/20" />
          <span className="size-2.5 rounded-full bg-canvas-white/20" />
        </span>
        <span className="ml-2 text-caption text-stone">deploy-on-merge</span>
        <span className="ml-auto flex items-center gap-1.5">
          <span
            className={
              cursor >= order.length
                ? "size-1.5 rounded-full bg-mint-green"
                : "size-1.5 animate-pulse rounded-full bg-phoenix-orange"
            }
            aria-hidden
          />
          <span className="text-caption text-stone">
            {cursor >= order.length ? "Succeeded" : cursor < 0 ? "Queued" : "Running"}
          </span>
        </span>
      </div>

      <div className="rounded-lg bg-[#1b1a19] p-6">
        <div
          className="relative mx-auto w-full"
          style={{ maxWidth: WIDTH, aspectRatio: `${WIDTH} / ${HEIGHT}` }}
        >
          <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} className="absolute inset-0 size-full" aria-hidden>
            {edges.map((edge) => {
              const from = steps.find((s) => s.id === edge.from)!;
              const to = steps.find((s) => s.id === edge.to)!;
              const x1 = from.x * COL + NODE_W;
              const y1 = from.y * ROW + NODE_H / 2;
              const x2 = to.x * COL;
              const y2 = to.y * ROW + NODE_H / 2;
              const untaken = edge.branch && edge.branch !== TAKEN;
              const flowed = cursor > order.indexOf(edge.from) && !untaken;

              return (
                <path
                  key={`${edge.from}-${edge.to}`}
                  d={`M ${x1} ${y1} C ${x1 + 44} ${y1}, ${x2 - 44} ${y2}, ${x2} ${y2}`}
                  fill="none"
                  strokeWidth={1.5}
                  strokeDasharray={untaken ? "4 4" : undefined}
                  className="transition-[stroke] duration-500"
                  stroke={flowed ? "#b7efb2" : "rgba(255,255,255,0.16)"}
                />
              );
            })}
          </svg>

          {steps.map((step) => {
            const status = statusOf(step.id);
            const palette = categories[step.category];
            return (
              <div
                key={step.id}
                className="absolute transition-opacity duration-500"
                style={{
                  width: `${(NODE_W / WIDTH) * 100}%`,
                  left: `${((step.x * COL) / WIDTH) * 100}%`,
                  top: `${((step.y * ROW) / HEIGHT) * 100}%`,
                  opacity: status === "pending" ? 0.35 : status === "skipped" ? 0.25 : 1,
                }}
              >
                <div
                  className="rounded-lg px-2.5 py-2 transition-all duration-500"
                  style={{
                    background: status === "pending" || status === "skipped" ? "#232221" : palette.hex,
                    outline: status === "running" ? "2px solid #e8400d" : "1px solid rgba(255,255,255,0.08)",
                    outlineOffset: status === "running" ? "2px" : "0",
                  }}
                >
                  <span
                    className="block text-[9px] uppercase tracking-[0.3px]"
                    style={{
                      color: status === "pending" || status === "skipped" ? "#6d6c6b" : "rgba(17,17,17,0.55)",
                    }}
                  >
                    {status === "skipped" ? "Skipped" : palette.label}
                  </span>
                  <span
                    className="mt-0.5 block truncate text-caption"
                    style={{
                      color: status === "pending" || status === "skipped" ? "#8a8a88" : "#111111",
                    }}
                  >
                    {step.label}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
