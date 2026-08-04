"use client";

import Link from "next/link";
import { useState } from "react";

import { AuthFields, AuthLayout } from "@/components/auth-form";
import { enterApp, signIn } from "@/lib/auth-client";

export default function LoginPage() {
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setPending(true);

    const form = new FormData(event.currentTarget);
    const result = await signIn.email({
      email: String(form.get("email")),
      password: String(form.get("password")),
    });

    if (result.error) {
      setPending(false);
      setError(result.error.message ?? "Sign in failed");
      return;
    }
    // Deliberately leaves pending set: the page is on its way out, and
    // flipping the button back to "Sign in" first reads as a failure.
    enterApp();
  }

  return (
    <AuthLayout
      title="Sign in"
      subtitle="Pick up where you left off."
      footer={
        <>
          No account?{" "}
          <Link href="/signup" className="text-ink underline underline-offset-4">
            Create one
          </Link>
        </>
      }
    >
      <form onSubmit={onSubmit}>
        <AuthFields mode="login" error={error} pending={pending} submitLabel="Sign in" />
      </form>
    </AuthLayout>
  );
}
