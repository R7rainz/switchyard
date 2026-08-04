"use client";

import { useRouter } from "next/navigation";
import { useEffect, type ReactNode } from "react";

import { Splash } from "@/components/ui";
import { useSession } from "@/lib/auth-client";

/**
 * The builder's guard, and nothing else.
 *
 * It renders no AppShell: the canvas is full-bleed with a header of its own,
 * and the centred 1200px shell would box it and stack two headers. The
 * signed-in screens that do want the shell live in the (app) route group.
 */
export default function BuilderLayout({ children }: { children: ReactNode }) {
  const router = useRouter();
  const { data: session, isPending } = useSession();

  useEffect(() => {
    if (!isPending && !session) router.replace("/login");
  }, [isPending, session, router]);

  if (isPending || !session) return <Splash />;

  return <>{children}</>;
}
