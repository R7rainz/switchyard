"use client";

import { useRouter } from "next/navigation";
import { useEffect, type ReactNode } from "react";

import { AppShell } from "@/components/app-shell";
import { PageTransition } from "@/components/page-transition";
import { Splash } from "@/components/ui";
import { useSession } from "@/lib/auth-client";

/**
 * The shell every full-width signed-in screen sits inside.
 *
 * A route group rather than a layout per section, and that is the point: when
 * /workflows and /settings each rendered their own AppShell, moving between
 * them unmounted the header and built a new one, so the chrome flickered on
 * every navigation. One layout above both means React keeps the header mounted
 * and only swaps what is under it.
 *
 * The builder is deliberately outside this group — it is a full-bleed canvas
 * with its own header, and it would be boxed by the centred shell.
 */
export default function AppLayout({ children }: { children: ReactNode }) {
  const router = useRouter();
  const { data: session, isPending } = useSession();

  useEffect(() => {
    if (!isPending && !session) router.replace("/login");
  }, [isPending, session, router]);

  if (isPending || !session) return <Splash />;

  return (
    <AppShell>
      <PageTransition>{children}</PageTransition>
    </AppShell>
  );
}
