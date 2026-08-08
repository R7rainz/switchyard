"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { GitBranch, KeyRound, Network, Radio } from "lucide-react";
import { useEffect } from "react";

import { Capabilities } from "@/components/capabilities";
import { PhoenixGradient } from "@/components/gradient";
import { HeroGraph } from "@/components/hero-graph";
import { Pricing } from "@/components/pricing";
import { RunRecord } from "@/components/run-record";
import { Reveal } from "@/components/reveal";
import { Badge, Button, Eyebrow, Splash, Wordmark } from "@/components/ui";
import { useSession } from "@/lib/auth-client";

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

        <div className="relative mx-auto max-w-[1200px] px-4 sm:px-6">
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

          <div className="grid items-center gap-12 py-14 sm:py-20 lg:grid-cols-[minmax(0,420px)_minmax(0,1fr)] lg:gap-16 lg:py-24">
            <div className="flex flex-col items-start gap-7">
            <Badge>
              <span aria-hidden className="size-1.5 rounded-full bg-phoenix-orange" />
              Workflow automation for engineers
            </Badge>

            {/* Weight 900 uppercase at display size: the poster moment, used
                once on the whole site. Everything else is weight 400. */}
            <h1 className="text-heading font-black uppercase text-ink [word-spacing:0.12em] sm:text-heading-lg">
              AI drafts the route.
              <br />
              You throw the switches.
            </h1>

            <p className="max-w-md text-body-sm leading-relaxed text-ink/70 sm:text-body">
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
              <p className="text-caption text-ink/55">No black-box execution · bring your own AI keys</p>
            </div>

            {/* The product, running. DESIGN.md's imagery guidance is UI-in-UI:
                showing the thing is the picture. Everything this panel does —
                dependency order, a branch taken, the other recorded skipped —
                is behaviour the engine actually has. */}
            <HeroGraph />
          </div>

          <div className="grid border-t border-ink/10 sm:grid-cols-2 lg:grid-cols-4">
            {[
              { icon: Network, title: "Visual graph", detail: "Edit every generated step" },
              { icon: GitBranch, title: "Real triggers", detail: "GitHub, schedules, webhooks" },
              { icon: Radio, title: "Live execution", detail: "Watch each node resolve" },
              { icon: KeyRound, title: "Your providers", detail: "OpenAI, Gemini, Anthropic" },
            ].map((item) => {
              const Icon = item.icon;
              return (
                <div key={item.title} className="flex items-center gap-3 border-b border-ink/10 py-4 sm:px-4 sm:first:pl-0 sm:[&:nth-child(odd)]:border-r lg:border-b-0 lg:border-r lg:last:border-r-0">
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-canvas-white/55 backdrop-blur"><Icon size={16} strokeWidth={1.7} /></span>
                  <span className="min-w-0"><span className="block text-body-sm text-ink">{item.title}</span><span className="block text-caption text-ink/55">{item.detail}</span></span>
                </div>
              );
            })}
          </div>
        </div>
      </section>

      {/* Light band after the gradient — the alternating rhythm the system asks
          for, at application distance rather than 120px marketing gaps. */}
      <section className="mx-auto max-w-[1200px] px-4 py-16 sm:px-6 sm:py-20">
        <Reveal>
          <Eyebrow>What it runs</Eyebrow>
          <h2 className="mt-4 max-w-2xl text-heading-sm text-ink sm:text-heading">
            Every node is one you can open, read, and change.
          </h2>
        </Reveal>

        <Capabilities />
      </section>

      <Pricing />

      {/* The dark counterweight. One per page, at the bottom, as the closer.
          Split, because the claim it makes is about a record — so it shows one
          rather than leaving half the band empty. */}
      <section className="bg-charcoal">
        <div className="mx-auto grid max-w-[1200px] items-center gap-12 px-4 py-20 sm:px-6 sm:py-24 lg:grid-cols-2 lg:gap-16">
          <Reveal className="flex flex-col items-start gap-8">
            <h2 className="max-w-xl text-heading-sm text-canvas-white sm:text-heading">
              Deterministic runs. Nothing hidden.
            </h2>
            <p className="max-w-md text-body-sm leading-relaxed text-stone">
              A saved workflow executes the same way until you change it. Every run keeps a copy of
              the graph it executed, so what you see later is what actually happened — including
              which node failed and what it said.
            </p>
            <Link href="/signup">
              <Button variant="neutral" className="h-12 px-6">
                Start building
              </Button>
            </Link>
          </Reveal>

          <Reveal delay={120}>
            <RunRecord />
          </Reveal>
        </div>
      </section>
    </main>
  );
}
