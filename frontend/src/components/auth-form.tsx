"use client";

import Link from "next/link";
import type { ReactNode } from "react";

import { Button, ErrorNote, Field, Input, Mono, StatusDot } from "./ui";

/**
 * The shell both auth screens sit in.
 *
 * A single light card on the black canvas — the signature figure/ground move,
 * used here because the form is the only object in the room.
 */
export function AuthLayout({
  title,
  children,
  footer,
}: {
  title: string;
  children: ReactNode;
  footer: ReactNode;
}) {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-8 px-6 py-16">
      <Link href="/" className="flex items-center gap-2">
        <StatusDot tone="live" />
        <Mono className="tracking-[0.08em] text-bone">Switchyard</Mono>
      </Link>

      <div className="w-full max-w-sm rounded-lg border border-carbon-lift p-8">
        <h1 className="mb-8 text-heading text-bone">{title}</h1>
        {children}
      </div>

      <p className="text-body-sm text-warm-granite">{footer}</p>
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
    <div className="flex flex-col gap-6">
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

      <Button type="submit" variant="light" disabled={pending} className="w-full">
        {pending ? "Working…" : submitLabel}
      </Button>
    </div>
  );
}
