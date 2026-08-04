"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { Button, Mono, Splash, StatusDot } from "@/components/ui";
import { useSession } from "@/lib/auth-client";

export default function Home() {
  const router = useRouter();
  const { data: session, isPending } = useSession();

  // A signed-in visitor wants the app, not the pitch.
  useEffect(() => {
    if (session) router.replace("/workflows");
  }, [session, router]);

  // Never an empty element: a blank black screen reads as a page that failed
  // to load, not one that is still deciding.
  if (isPending || session) return <Splash />;

  return (
    <main className="mx-auto flex min-h-screen max-w-[1200px] flex-col justify-center px-6 py-20">
      <div className="flex max-w-2xl flex-col gap-8">
        <span className="flex items-center gap-2">
          <StatusDot tone="live" />
          <Mono>Workflow automation for engineers</Mono>
        </span>

        {/* Authority comes from size and tracking, never from weight. */}
        <h1 className="text-heading-lg text-bone sm:text-display">
          AI drafts the route.
          <br />
          You throw the switches.
        </h1>

        <p className="max-w-md text-body text-warm-granite">
          Describe a workflow in plain language. Switchyard turns it into a graph you can read,
          edit, and run — and shows you exactly what happened when it did.
        </p>

        <div className="flex flex-wrap items-center gap-3">
          <Link href="/signup">
            <Button variant="light">Create an account</Button>
          </Link>
          <Link href="/login">
            <Button variant="ghost">Sign in</Button>
          </Link>
        </div>
      </div>
    </main>
  );
}
