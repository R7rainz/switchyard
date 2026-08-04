"use client";

import { categories } from "@/lib/categories";

import { Reveal } from "./reveal";

/**
 * The four node families, each demonstrating itself.
 *
 * The section used to state what a trigger, a condition, an AI node, and an
 * HTTP node do, in prose, on four flat colour tiles. That is the gap the hero
 * closed and this one did not: the design was carrying none of the meaning.
 *
 * Each card now runs the behaviour it is describing. All of it is drawn in Ink
 * at low alpha on the pastel, because the flat colour is the elevation — a
 * white inner panel would turn a taxonomy tile into a card with a card on it.
 */

const ink = (alpha: number) => `rgba(17, 17, 17, ${alpha})`;

/** A trigger fires: a pulse leaves the node and travels away down the edge. */
function TriggerDemo() {
  return (
    <Demo>
      <Chip label="Manual" />
      <Wire />
      <Pulse />
      <Chip label="Run" faint />
    </Demo>
  );
}

/** A condition takes one path at a time, and the other visibly dims. */
function LogicDemo() {
  return (
    <Demo>
      <Chip label="If" />
      <div className="relative flex h-full flex-1 flex-col justify-center gap-1.5 pl-3">
        <span className="flex items-center gap-2 animate-[var(--animate-branch-a)]">
          <span className="h-px w-4" style={{ background: ink(0.35) }} />
          <Chip label="true" small />
        </span>
        <span className="flex items-center gap-2 animate-[var(--animate-branch-b)]">
          <span className="h-px w-4" style={{ background: ink(0.35) }} />
          <Chip label="false" small />
        </span>
      </div>
    </Demo>
  );
}

/** An AI node answers: the reply forms line by line. */
function AIDemo() {
  return (
    <Demo>
      <Chip label="Prompt" />
      <div className="flex flex-1 flex-col gap-1.5 pl-3">
        {[0, 1, 2].map((line) => (
          <span
            key={line}
            className="h-1.5 origin-left rounded-full animate-[var(--animate-emit)]"
            style={{
              background: ink(0.28),
              // Ragged, so it reads as text rather than a loading bar.
              width: ["100%", "76%", "88%"][line],
              animationDelay: `${line * 220}ms`,
            }}
          />
        ))}
      </div>
    </Demo>
  );
}

/** An HTTP node calls out and something comes back — including when it fails. */
function HTTPDemo() {
  return (
    <Demo>
      <Chip label="GET" />
      <Wire />
      <Pulse />
      <span
        className="rounded-full px-2 py-1 text-[10px] animate-[var(--animate-arrive)]"
        style={{ background: ink(0.82), color: "#fff" }}
      >
        200
      </span>
    </Demo>
  );
}

function Demo({ children }: { children: React.ReactNode }) {
  return <div className="flex h-16 items-center">{children}</div>;
}

function Chip({ label, faint, small }: { label: string; faint?: boolean; small?: boolean }) {
  return (
    <span
      className={small ? "rounded px-1.5 py-0.5 text-[10px]" : "rounded-md px-2 py-1.5 text-[11px]"}
      style={{
        background: ink(faint ? 0.06 : 0.1),
        color: ink(faint ? 0.4 : 0.75),
        boxShadow: `inset 0 0 0 1px ${ink(0.08)}`,
      }}
    >
      {label}
    </span>
  );
}

function Wire() {
  return <span className="h-px flex-1" style={{ background: ink(0.2) }} />;
}

/**
 * The dot that crosses a wire. Absolutely placed so its travel is a transform
 * and never nudges the row it sits in.
 */
function Pulse() {
  return (
    <span className="relative -ml-px w-0">
      <span
        className="absolute top-1/2 size-1.5 -translate-y-1/2 rounded-full animate-[var(--animate-travel)]"
        style={{ background: "#e8400d", ["--travel-distance" as string]: "-56px" }}
      />
    </span>
  );
}

const cards = [
  {
    category: "trigger",
    title: "Triggers",
    body: "Manual, webhook, or a schedule. Exactly one starts a run.",
    demo: <TriggerDemo />,
  },
  {
    category: "logic",
    title: "Logic",
    body: "A condition sends the run down one path. The other is recorded skipped, not left pending.",
    demo: <LogicDemo />,
  },
  {
    category: "ai",
    title: "AI",
    body: "Ask a model mid-run. Its answer flows into the node after it.",
    demo: <AIDemo />,
  },
  {
    category: "http",
    title: "HTTP",
    body: "Call anything. Status and body come back — kept even when the call fails.",
    demo: <HTTPDemo />,
  },
] as const;

export function Capabilities() {
  return (
    <div className="mt-12 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      {cards.map((card, index) => (
        // Staggered by index so the row arrives as a sequence. Small enough
        // that the last card is not still waiting when the first is read.
        <Reveal key={card.title} delay={index * 70}>
          <div
            className="flex h-full flex-col gap-5 rounded-xl p-5"
            style={{ background: categories[card.category].hex }}
          >
            {card.demo}
            {/* Not bottom-aligned. The demo is a fixed height, so letting the
                text follow it directly keeps every title on the same line —
                mt-auto pushed them apart by however long each body happened
                to be. */}
            <div className="flex flex-col gap-2">
              <span className="text-body-lg text-ink">{card.title}</span>
              <span className="text-body-sm leading-relaxed text-ink/65">{card.body}</span>
            </div>
          </div>
        </Reveal>
      ))}
    </div>
  );
}
