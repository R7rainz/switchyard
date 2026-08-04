"use client";

import { usePathname, useRouter } from "next/navigation";
import { useEffect, type ReactNode } from "react";

import { AppShell } from "@/components/app-shell";
import { useSession } from "@/lib/auth-client";
import { Splash } from "@/components/ui";

/**
 * The guard for everything behind sign-in.
 *
 * Client-side because the session lives in a Better Auth cookie the browser
 * holds; the Go API is the thing that actually enforces access, and it checks
 * a verified JWT per request. This only decides which screen to draw.
 */
export default function WorkflowsLayout({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { data: session, isPending } = useSession();

  useEffect(() => {
    if (!isPending && !session) router.replace("/login");
  }, [isPending, session, router]);

  if (isPending || !session) return <Splash />;

  // The builder is a full-bleed canvas with its own header. Wrapping it in the
  // centred 1200px shell would box the canvas and stack two headers.
  if (pathname !== "/workflows") return <>{children}</>;

  return <AppShell>{children}</AppShell>;
}
