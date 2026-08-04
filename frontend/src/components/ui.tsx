/**
 * The system's primitives.
 *
 * Everything here encodes a rule from DESIGN.md so no page has to remember it:
 * radii are 3px on controls and 10px on cards, chromatic colour is never a
 * fill, there are no shadows, and weight stays at 400. A page that needs a
 * variant should get one here rather than reaching for arbitrary classes —
 * that is how a system turns into a pile of one-offs.
 */
import type { ComponentProps, ReactNode } from "react";

export function cx(...parts: Array<string | false | null | undefined>) {
  return parts.filter(Boolean).join(" ");
}

/**
 * The instrument voice. Mono 12px uppercase marks a system surface — column
 * headers, status tags, eyebrows — and its job is to be distinguishable from
 * page copy at a glance.
 */
export function Mono({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      className={cx(
        "font-mono text-caption uppercase text-warm-granite",
        className,
      )}
      {...props}
    />
  );
}

type ButtonVariant = "primary" | "ghost" | "light" | "danger";

const buttonVariants: Record<ButtonVariant, string> = {
  // Neutral dark fill. Chromatic CTAs would break the monochrome chrome.
  primary: "bg-carbon-lift text-bone hover:bg-[#272423] border border-transparent",
  // A typographic button: no fill ever appears, only text and border shift.
  ghost: "bg-transparent text-bone border border-ash-stroke hover:border-chalk hover:text-chalk",
  // The one high-emphasis light control, used sparingly.
  light: "bg-chalk text-obsidian-canvas border border-transparent hover:bg-bone",
  // Destructive is still not a filled colour — the orange is a signal, and it
  // says so on the border and the text where it cannot be mistaken for chrome.
  danger:
    "bg-transparent text-warm-granite border border-ash-stroke hover:border-signal-orange hover:text-signal-orange",
};

export function Button({
  variant = "primary",
  className,
  ...props
}: ComponentProps<"button"> & { variant?: ButtonVariant }) {
  return (
    <button
      className={cx(
        "inline-flex h-9 items-center justify-center gap-2 rounded-sm px-3.5",
        "text-body-sm whitespace-nowrap cursor-pointer",
        "disabled:cursor-not-allowed disabled:opacity-40",
        buttonVariants[variant],
        className,
      )}
      {...props}
    />
  );
}

/**
 * A dark card: the border is the card, not a fill. Depth comes from contrast
 * and spacing, never from a shadow.
 */
export function Card({ className, ...props }: ComponentProps<"div">) {
  return <div className={cx("rounded-lg border border-carbon-lift p-6", className)} {...props} />;
}

/**
 * The signature move — the one bright object on near-black ground. Reserved
 * for the thing the page is actually about, or it stops meaning anything.
 */
export function LightCard({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cx("rounded-lg bg-bone p-6 text-obsidian-canvas", className)}
      {...props}
    />
  );
}

export function Input({ className, ...props }: ComponentProps<"input">) {
  return (
    <input
      className={cx(
        "h-9 w-full rounded-sm border border-ash-stroke bg-transparent px-3",
        "text-body-sm text-bone placeholder:text-graphite-mid",
        "focus:border-warm-granite focus:outline-none",
        className,
      )}
      {...props}
    />
  );
}

export function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return (
    <textarea
      className={cx(
        "w-full resize-y rounded-sm border border-ash-stroke bg-transparent px-3 py-2",
        "text-body-sm text-bone placeholder:text-graphite-mid",
        "focus:border-warm-granite focus:outline-none",
        className,
      )}
      {...props}
    />
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-2">
      <Mono>{label}</Mono>
      {children}
    </label>
  );
}

/**
 * A 6px dot before a label. Orange is live, green is a good outcome, granite is
 * anything at rest — the colour is the data, which is the only thing colour is
 * allowed to be here.
 */
export function StatusDot({ tone = "idle" }: { tone?: "live" | "good" | "bad" | "idle" }) {
  const tones = {
    live: "bg-signal-orange",
    good: "bg-metric-green",
    bad: "bg-signal-orange",
    idle: "bg-graphite-mid",
  };
  return (
    <span
      aria-hidden
      className={cx("inline-block size-1.5 shrink-0 rounded-full", tones[tone])}
    />
  );
}

/** A failure the user can read. Never a toast that disappears before it is. */
export function ErrorNote({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="flex items-start gap-2 text-body-sm text-signal-orange">
      <StatusDot tone="bad" />
      <span className="-mt-0.5">{children}</span>
    </p>
  );
}

export function EmptyState({
  title,
  hint,
  action,
}: {
  title: string;
  hint: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-4 rounded-lg border border-carbon-lift px-6 py-20 text-center">
      <p className="text-body text-bone">{title}</p>
      <p className="max-w-sm text-body-sm text-warm-granite">{hint}</p>
      {action}
    </div>
  );
}

/**
 * A skeleton rather than a spinner: the page keeps its shape while it loads, so
 * nothing jumps when the data lands.
 */
export function Skeleton({ className }: { className?: string }) {
  return <div className={cx("animate-pulse rounded-sm bg-carbon-lift", className)} />;
}

/**
 * What the screen shows while it works out whether you are signed in.
 *
 * Never nothing. Returning an empty element leaves a black void that is
 * indistinguishable from a page that failed to load — which is exactly what it
 * looked like the first time this was screenshotted.
 */
export function Splash() {
  return (
    <div className="flex min-h-screen items-center justify-center gap-2">
      <StatusDot tone="live" />
      <Mono className="tracking-[0.08em] text-graphite-mid">Switchyard</Mono>
    </div>
  );
}
