"use client";

import Link from "next/link";
import type { ReactNode } from "react";

import { PhoenixGradient } from "./gradient";
import { Button, ErrorNote, Field, Input, Wordmark } from "./ui";

/**
 * The shell both auth screens sit in.
 *
 * Split: the gradient owns the left half on a wide screen and the form sits on
 * plain canvas. That keeps the one sanctioned gradient to a brand moment rather
 * than putting colour behind a form somebody has to read.
 */
export function AuthLayout({
  title,
  subtitle,
  children,
  footer,
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
  footer: ReactNode;
}) {
  return (
    <main className="flex min-h-screen">
      <div className="relative hidden w-[42%] shrink-0 overflow-hidden lg:block">
        <PhoenixGradient className="absolute inset-0" />
        <div className="relative flex h-full flex-col justify-between p-10">
          <Link href="/">
            <Wordmark />
          </Link>
          <p className="max-w-xs text-body-lg text-ink">
            AI drafts the route. You throw the switches.
          </p>
        </div>
      </div>

      <div className="flex flex-1 flex-col items-center justify-center px-6 py-16">
        <div className="w-full max-w-sm">
          <Link href="/" className="lg:hidden">
            <Wordmark className="mb-10" />
          </Link>

          <h1 className="text-heading-sm text-ink">{title}</h1>
          <p className="mt-3 mb-10 text-body-sm text-ash">{subtitle}</p>

          {children}

          <p className="mt-8 text-body-sm text-ash">{footer}</p>
        </div>
      </div>
    </main>
  );
}

export function AuthFields({
  mode,
  error,
  pending,
  submitLabel,
}: {
  mode: "login" | "signup";
  error: string | null;
  pending: boolean;
  submitLabel: string;
}) {
  return (
    <div className="flex flex-col gap-5">
      {mode === "signup" && (
        <Field label="Name">
          <Input name="name" required autoComplete="name" placeholder="Ada Lovelace" />
        </Field>
      )}

      <Field label="Email">
        <Input
          name="email"
          type="email"
          required
          autoComplete="email"
          placeholder="you@example.com"
        />
      </Field>

      <Field label="Password">
        <Input
          name="password"
          type="password"
          required
          minLength={8}
          autoComplete={mode === "login" ? "current-password" : "new-password"}
          placeholder="At least 8 characters"
        />
      </Field>

      {error && <ErrorNote>{error}</ErrorNote>}

      <Button type="submit" disabled={pending} className="mt-1 h-12 w-full">
        {pending ? "Working…" : submitLabel}
      </Button>
    </div>
  );
}
