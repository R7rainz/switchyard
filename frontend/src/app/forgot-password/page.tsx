"use client";

import Link from "next/link";
import { useState } from "react";

import { AuthLayout } from "@/components/auth-form";
import { Button, ErrorNote, Field, Input } from "@/components/ui";
import { authClient } from "@/lib/auth-client";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [sent, setSent] = useState(false);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setPending(true);

    const result = await authClient.requestPasswordReset({
      email,
      redirectTo: `${window.location.origin}/reset-password`,
    });

    setPending(false);
    if (result.error) {
      setError(result.error.message ?? "Could not send reset instructions");
      return;
    }
    setSent(true);
  }

  return (
    <AuthLayout
      title="Forgot password"
      subtitle="We’ll help you get back into your account."
      footer={
        <>
          Remembered it?{" "}
          <Link href="/login" className="text-ink underline underline-offset-4">
            Sign in
          </Link>
        </>
      }
    >
      {sent ? (
        <div className="flex flex-col gap-5">
          <p className="text-body-sm leading-relaxed text-ash">
            If an account exists for that email, reset instructions are on their way. In local
            development, the reset link is printed in the frontend logs.
          </p>
          <Link href="/login" className="text-body-sm text-ink underline underline-offset-4">
            Back to sign in
          </Link>
        </div>
      ) : (
        <form onSubmit={onSubmit} className="flex flex-col gap-5">
          <Field label="Email">
            <Input
              type="email"
              required
              autoComplete="email"
              placeholder="you@example.com"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </Field>

          {error && <ErrorNote>{error}</ErrorNote>}

          <Button type="submit" disabled={pending} className="h-12 w-full">
            {pending ? "Sending…" : "Send reset link"}
          </Button>
        </form>
      )}
    </AuthLayout>
  );
}
