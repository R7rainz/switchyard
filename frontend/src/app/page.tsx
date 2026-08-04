"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { PhoenixGradient } from "@/components/gradient";
import { Badge, Button, Eyebrow, PastelCard, Splash, Wordmark } from "@/components/ui";
import { useSession } from "@/lib/auth-client";

/**
 * The five pastels are a taxonomy, not decoration. Here they name the node
 * families the engine actually ships, so the colour a capability wears on this
 * page is the colour it wears on the canvas.
 */
const capabilities = [
  { tone: "canary", title: "Triggers", body: "Manual, webhook, or a schedule. Exactly one starts a run." },
  { tone: "mint", title: "Logic", body: "Conditions split the run down a named branch, and the path not taken is recorded." },
  { tone: "violet", title: "AI", body: "Ask a model mid-run. Its answer flows into the next node." },
  { tone: "pink", title: "HTTP", body: "Call anything. Status and body come back, even when it fails." },
] as const;

export default function Home() {
  const router = useRouter();
  const { data: session, isPending } = useSession();

  // A signed-in visitor wants the app, not the pitch.
  useEffect(() => {
    if (session) router.replace("/workflows");
  }, [session, router]);

  // Never an empty element: a blank screen reads as a page that failed to load,
  // not one that is still deciding.
  if (isPending || session) return <Splash />;

  return (
    <main>
      {/* The one gradient the system allows, and the only animated surface in
          the product. It appears here and on the auth split, nowhere else. */}
      <section className="relative overflow-hidden">
        <PhoenixGradient className="absolute inset-0" />

        <div className="relative mx-auto max-w-[1200px] px-6">
          <nav className="flex h-[62px] items-center justify-between">
            <Wordmark />
            <div className="flex items-center gap-2">
              <Link href="/login">
                <Button variant="ghost">Sign in</Button>
              </Link>
              <Link href="/signup">
                <Button>Get started</Button>
              </Link>
            </div>
          </nav>

          <div className="flex flex-col items-start gap-8 py-24 sm:py-32">
            <Badge>
              <span aria-hidden className="size-1.5 rounded-full bg-phoenix-orange" />
              Workflow automation for engineers
            </Badge>

            {/* Weight 900 uppercase at display size: the poster moment, used
                once on the whole site. Everything else is weight 400. */}
            <h1 className="max-w-4xl text-heading-lg font-black uppercase text-ink sm:text-display">
              AI drafts the route.
              <br />
              You throw the switches.
            </h1>

            <p className="max-w-md text-body-lg text-ink/70">
              Describe a workflow in plain language. Switchyard turns it into a graph you can read,
              edit, and run — and shows you exactly what happened when it did.
            </p>

            <div className="flex flex-wrap items-center gap-3">
              <Link href="/signup">
                <Button className="h-12 px-6">Create an account</Button>
              </Link>
              <Link href="/login">
                <Button variant="neutral" className="h-12 px-6">
                  Sign in
                </Button>
              </Link>
            </div>
          </div>
        </div>
      </section>

      {/* Light band after the gradient — the alternating rhythm the system asks
          for, at application distance rather than 120px marketing gaps. */}
      <section className="mx-auto max-w-[1200px] px-6 py-20">
        <Eyebrow>What it runs</Eyebrow>
        <h2 className="mt-4 max-w-2xl text-heading-sm text-ink sm:text-heading">
          Every node is one you can open, read, and change.
        </h2>

        <div className="mt-12 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {capabilities.map((item) => (
            <PastelCard key={item.title} tone={item.tone} className="flex min-h-44 flex-col justify-between gap-6">
              <span className="text-body-lg">{item.title}</span>
              <span className="text-body-sm leading-relaxed text-ink/70">{item.body}</span>
            </PastelCard>
          ))}
        </div>
      </section>

      {/* The dark counterweight. One per page, at the bottom, as the closer. */}
      <section className="bg-charcoal">
        <div className="mx-auto flex max-w-[1200px] flex-col items-start gap-8 px-6 py-24">
          <h2 className="max-w-2xl text-heading-sm text-canvas-white sm:text-heading">
            Deterministic runs. Nothing hidden.
          </h2>
          <p className="max-w-md text-body-sm leading-relaxed text-stone">
            A saved workflow executes the same way until you change it. Every run keeps a copy of the
            graph it executed, so what you see later is what actually happened.
          </p>
          <Link href="/signup">
            <Button variant="neutral" className="h-12 px-6">
              Start building
            </Button>
          </Link>
        </div>
      </section>
    </main>
  );
}
