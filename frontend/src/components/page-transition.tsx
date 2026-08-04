"use client";

import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

/**
 * Fades and lifts the page in when the route changes.
 *
 * Only the content moves. The header is mounted by the layout above this, so it
 * stays put while what is under it changes — which is why the shell was hoisted
 * into a route group.
 *
 * A key and a CSS animation, deliberately, with no effect and no state. The
 * first version drove it from state: set shown=false, schedule a frame to set
 * it true, cancel that frame on cleanup. React runs mount -> cleanup -> mount
 * in development, so the frame was cancelled and the second run hit a ref guard
 * that returned early — leaving every page rendered at opacity 0 forever. The
 * workflows screen was blank until you navigated somewhere else and back,
 * because only a pathname change re-ran the effect.
 *
 * Changing the key remounts this element and a CSS animation just runs on
 * mount. Nothing to schedule, nothing to cancel, no second render to skip.
 */
export function PageTransition({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  return (
    <div key={pathname} className="animate-[var(--animate-page-in)]">
      {children}
    </div>
  );
}
