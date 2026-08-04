"use client";

import { usePathname } from "next/navigation";
import { useEffect, useRef, useState, type ReactNode } from "react";

/**
 * Fades and lifts the page in when the route changes.
 *
 * Only the content moves. The header is mounted by the layout above this, so
 * it stays put while what is under it changes — which is the whole reason the
 * shell was hoisted into a route group.
 *
 * Transform and opacity only: DESIGN.md rules out animating layout properties,
 * and anything that reflows on a route change would be visible as a jump
 * rather than a transition.
 */
export function PageTransition({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const [shown, setShown] = useState(false);
  // What has already been animated, so a re-render for any other reason does
  // not replay the entrance.
  const settled = useRef<string | null>(null);

  useEffect(() => {
    if (settled.current === pathname) return;
    settled.current = pathname;
    setShown(false);
    // Next frame, so the browser paints the start of the transition before the
    // end of it. Setting both in one frame is a change with nothing to
    // interpolate, and no animation happens at all.
    const frame = requestAnimationFrame(() => setShown(true));
    return () => cancelAnimationFrame(frame);
  }, [pathname]);

  return (
    <div
      className={
        "transition-[opacity,transform] duration-300 [transition-timing-function:cubic-bezier(0.23,1,0.32,1)] motion-reduce:transition-none " +
        (shown ? "translate-y-0 opacity-100" : "translate-y-2 opacity-0")
      }
    >
      {children}
    </div>
  );
}
