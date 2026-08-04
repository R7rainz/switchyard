/**
 * The system's primitives.
 *
 * Each one encodes a rule from DESIGN.md so no page has to remember it: 8px on
 * controls and 12px on cards and nothing else, weight 400 even at heading
 * sizes, hairline borders rather than drop shadows on white, and the pastel
 * palette used only as a flat taxonomy fill. A page needing a variant should
 * get one here — that is how a system avoids becoming a pile of one-offs.
 */
import type { ComponentProps, ReactNode } from "react";

import { Logo } from "./logo";

export function cx(...parts: Array<string | false | null | undefined>) {
  return parts.filter(Boolean).join(" ");
}

/**
 * The 10px uppercase label above a title. Positive tracking, which is the one
 * place this system tracks outwards — everything larger tracks in.
 */
export function Eyebrow({ className, ...props }: ComponentProps<"span">) {
  return <span className={cx("text-eyebrow uppercase text-ash", className)} {...props} />;
}

type ButtonVariant = "primary" | "neutral" | "ghost" | "danger";

const buttonVariants: Record<ButtonVariant, string> = {
  // Ink, not a brand colour. Phoenix orange is a brand moment, never a fill.
  primary: "bg-ink text-canvas-white hover:bg-charcoal",
  // More presence than ghost, less than primary.
  neutral: "bg-pearl text-ink hover:bg-stone/40",
  ghost: "bg-transparent text-ink hover:bg-pearl",
  // Destructive states the risk in the text, and only tints on hover.
  danger: "bg-transparent text-ash hover:bg-phoenix-orange/10 hover:text-phoenix-orange",
};

export function Button({
  variant = "primary",
  className,
  ...props
}: ComponentProps<"button"> & { variant?: ButtonVariant }) {
  return (
    <button
      className={cx(
        "inline-flex h-10 items-center justify-center gap-2 rounded-lg px-4",
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
 * A white card on white. A hairline border does the separating — a drop shadow
 * on a white background is the thing this system explicitly refuses.
 */
export function Card({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      className={cx("rounded-xl border border-hairline bg-canvas-white p-5", className)}
      {...props}
    />
  );
}

/** The five pastels are a taxonomy, not decoration: one per node category. */
export const pastels = {
  pink: "bg-petal-pink",
  mint: "bg-mint-green",
  canary: "bg-canary-yellow",
  violet: "bg-soft-violet",
  aqua: "bg-aqua",
} as const;

export type Pastel = keyof typeof pastels;

/**
 * A flat colour tile. No border, no shadow — the colour is the elevation, and
 * adding either would flatten the one thing that makes it read as a category.
 */
export function PastelCard({
  tone,
  className,
  ...props
}: ComponentProps<"div"> & { tone: Pastel }) {
  return <div className={cx("rounded-xl p-5 text-ink", pastels[tone], className)} {...props} />;
}

/** A pill. Small, flat, and the only element allowed a 9999px radius. */
export function Badge({
  tone,
  className,
  ...props
}: ComponentProps<"span"> & { tone?: Pastel }) {
  return (
    <span
      className={cx(
        "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-caption text-ink",
        tone ? pastels[tone] : "bg-pearl",
        className,
      )}
      {...props}
    />
  );
}

export function Input({ className, ...props }: ComponentProps<"input">) {
  return (
    <input
      className={cx(
        "h-12 w-full rounded-xl border border-hairline bg-canvas-white px-4",
        "text-body-sm text-ink placeholder:text-ash",
        "focus:border-ink/25 focus:outline-none",
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
        "w-full resize-y rounded-xl border border-hairline bg-canvas-white px-4 py-3",
        "text-body-sm text-ink placeholder:text-ash",
        "focus:border-ink/25 focus:outline-none",
        className,
      )}
      {...props}
    />
  );
}

export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="flex flex-col gap-2">
      <Eyebrow>{label}</Eyebrow>
      {children}
      {hint && <span className="text-caption leading-normal text-ash">{hint}</span>}
    </label>
  );
}

/** A failure the user can read. Never a toast that disappears before it is. */
export function ErrorNote({ children }: { children: ReactNode }) {
  return (
    <p
      role="alert"
      className="rounded-lg bg-phoenix-orange/10 px-3 py-2 text-body-sm text-phoenix-orange"
    >
      {children}
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
    <div className="flex flex-col items-center gap-4 rounded-xl bg-cream-wash px-6 py-20 text-center">
      <p className="text-subheading text-ink">{title}</p>
      <p className="max-w-md text-body-sm leading-relaxed text-ash">{hint}</p>
      {action}
    </div>
  );
}

/**
 * A skeleton rather than a spinner: the page keeps its shape while it loads, so
 * nothing jumps when the data lands.
 */
export function Skeleton({ className }: { className?: string }) {
  return <div className={cx("animate-pulse rounded-lg bg-pearl", className)} />;
}

/** The brand lockup: the keyswitch mark and the name. */
export function Wordmark({ className }: { className?: string }) {
  return (
    <span className={cx("inline-flex items-center gap-2", className)}>
      <Logo size={18} />
      <span className="text-body-sm tracking-[-0.2px] text-ink">Switchyard</span>
    </span>
  );
}

/**
 * What the screen shows while it works out whether you are signed in.
 *
 * Never nothing. An empty element is indistinguishable from a page that failed
 * to load, which is exactly what the previous version looked like.
 */
export function Splash() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <Wordmark className="animate-pulse" />
    </div>
  );
}
