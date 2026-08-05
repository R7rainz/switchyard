import Link from "next/link";

import { Button, Card, Eyebrow } from "./ui";
import { Reveal } from "./reveal";
import { WebGLCardGlow } from "./webgl-card-glow";

type Plan = {
  name: string;
  price: string;
  cadence: string;
  summary: string;
  features: string[];
  status: "Available" | "Planned";
  featured?: boolean;
};

const plans: Plan[] = [
  {
    name: "Open",
    price: "$0",
    cadence: "during early access",
    summary: "Build and run engineering workflows without a credit card.",
    status: "Available",
    features: [
      "Editable workflow graphs",
      "Manual, webhook, and scheduled triggers",
      "Bring your own provider keys",
      "Execution history and live logs",
    ],
  },
  {
    name: "Pro",
    price: "$29",
    cadence: "per workspace / month",
    summary: "More room for the workflows your team relies on every day.",
    status: "Planned",
    features: [
      "Everything in Open",
      "Higher AI and execution limits",
      "Generation feedback controls",
      "Priority support",
    ],
    featured: true,
  },
  {
    name: "Team",
    price: "$99",
    cadence: "per workspace / month",
    summary: "Shared automation for engineering and platform teams.",
    status: "Planned",
    features: [
      "Everything in Pro",
      "Shared workspaces and roles",
      "Audit exports",
      "Custom limits and support",
    ],
  },
];

export function Pricing() {
  return (
    <section id="pricing" className="bg-cream-wash px-6 py-20 sm:py-24" aria-labelledby="pricing-title">
      <div className="mx-auto max-w-[1200px]">
        <Reveal>
          <Eyebrow>Simple by design</Eyebrow>
          <h2 id="pricing-title" className="mt-4 max-w-2xl text-heading-sm text-ink sm:text-heading">
            Start free. Pay when the work earns it.
          </h2>
          <p className="mt-5 max-w-xl text-body-sm leading-relaxed text-ash sm:text-body">
            Switchyard is free during early access. Paid workspaces are planned for teams that need
            more capacity, controls, and support.
          </p>
        </Reveal>

        <div className="mt-12 grid gap-4 lg:grid-cols-3">
          {plans.map((plan, index) => (
            <Reveal key={plan.name} delay={index * 80} className="h-full">
              <Card
                className={`group relative flex h-full flex-col gap-7 overflow-hidden transition-[border-color,background-color,transform] duration-300 hover:-translate-y-1 focus-within:-translate-y-1 motion-reduce:transform-none ${
                  plan.featured
                    ? "border-ink/30 bg-soft-violet hover:border-ink/60"
                    : "bg-canvas-white hover:border-ink/25 hover:bg-cream-wash"
                }`}
              >
                <WebGLCardGlow />
                <span
                  aria-hidden
                  className="pointer-events-none absolute inset-x-5 top-0 h-0.5 origin-left scale-x-0 bg-phoenix-orange transition-transform duration-300 group-hover:scale-x-100 group-focus-within:scale-x-100 motion-reduce:transition-none"
                />
                <div className="relative z-[1] flex items-start justify-between gap-4">
                  <div>
                    <h3 className="text-body-lg tracking-[-0.4px] text-ink">{plan.name}</h3>
                    <p className="mt-4 text-subheading text-ink">{plan.price}</p>
                    <p className="mt-1 text-caption text-ash">{plan.cadence}</p>
                  </div>
                  <div className="flex flex-wrap justify-end gap-1.5">
                    {plan.featured && (
                      <span className="rounded-full bg-ink px-2.5 py-1 text-caption text-canvas-white">
                        Recommended
                      </span>
                    )}
                    <span
                      className={`rounded-full px-2.5 py-1 text-caption ${
                        plan.status === "Available" ? "bg-mint-green text-ink" : "bg-pearl text-ash"
                      }`}
                    >
                      {plan.status}
                    </span>
                  </div>
                </div>

                <p className="relative z-[1] min-h-12 text-body-sm leading-relaxed text-ash">{plan.summary}</p>

                <ul className="relative z-[1] flex flex-1 flex-col gap-3 text-body-sm text-ink">
                  {plan.features.map((feature) => (
                    <li key={feature} className="flex gap-2.5">
                      <span aria-hidden className="mt-1.5 size-1.5 shrink-0 rounded-full bg-phoenix-orange" />
                      <span>{feature}</span>
                    </li>
                  ))}
                </ul>

                <Link href="/signup" className="relative z-[1] block">
                  <Button variant={plan.featured ? "neutral" : "primary"} className="w-full">
                    {plan.status === "Available" ? "Start free" : "Join early access"}
                  </Button>
                </Link>
              </Card>
            </Reveal>
          ))}
        </div>

        <p className="mt-8 text-caption text-ash">
          Paid subscriptions are not active yet. No billing or payment details are collected.
        </p>
      </div>
    </section>
  );
}
