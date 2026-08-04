"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";

import { cx } from "./ui";

/**
 * Fades and lifts its children in when they scroll into view.
 *
 * IntersectionObserver rather than a scroll listener: the browser does the
 * work off the main thread, and a scroll handler recalculating positions is
 * the classic way a page starts to feel heavy.
 *
 * It reveals once and then stops observing. Content that re-animates every
 * time it passes the fold is a distraction on the second pass.
 */
export function Reveal({
  children,
  delay = 0,
  className,
}: {
  children: ReactNode;
  /** Stagger, in milliseconds. Enough to read as a sequence, not a queue. */
  delay?: number;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [shown, setShown] = useState(false);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;

    // A browser without the API shows everything rather than hiding content
    // behind a capability check. Deferred rather than set inline, since a
    // synchronous setState in an effect cascades a second render before paint.
    if (typeof IntersectionObserver === "undefined") {
      const settle = setTimeout(() => setShown(true), 0);
      return () => clearTimeout(settle);
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry.isIntersecting) return;
        setShown(true);
        observer.disconnect();
      },
      // A little before the edge, so the movement finishes as it arrives
      // rather than starting once it is already being read.
      { rootMargin: "0px 0px -10% 0px", threshold: 0.1 },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  return (
    <div
      ref={ref}
      className={cx(
        // The entrance curve DESIGN.md names, and transform/opacity only.
        "transition-[opacity,transform] duration-700 [transition-timing-function:cubic-bezier(0.23,1,0.32,1)] motion-reduce:transition-none",
        shown ? "translate-y-0 opacity-100" : "translate-y-4 opacity-0",
        className,
      )}
      style={{ transitionDelay: `${delay}ms` }}
    >
      {children}
    </div>
  );
}
