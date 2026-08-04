"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";

import { AuthFields, AuthLayout } from "@/components/auth-form";
import { enterApp, signUp } from "@/lib/auth-client";

export default function SignupPage() {
  // Suspense because useSearchParams opts the page into client rendering, and
  // Next requires the boundary for it during prerender.
  return (
    <Suspense fallback={null}>
      <SignupPageForm />
    </Suspense>
  );
}

function SignupPageForm() {
  // Where to land afterwards. An invite link sends people through here and
  // needs them back, so the destination travels rather than being assumed.
  const next = useSearchParams().get("next");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setPending(true);

    const form = new FormData(event.currentTarget);
    const result = await signUp.email({
      name: String(form.get("name")),
      email: String(form.get("email")),
      password: String(form.get("password")),
    });

    if (result.error) {
      setPending(false);
      setError(result.error.message ?? "Sign up failed");
      return;
    }
    // Better Auth signs the new user in, and listing workspaces creates their
    // personal one, so the dashboard is ready by the time they arrive.
    enterApp(next ?? undefined);
  }

  return (
    <AuthLayout
      title="Create an account"
      subtitle="Free while Switchyard is in development."
      footer={
        <>
          Already have one?{" "}
          <Link href="/login" className="text-ink underline underline-offset-4">
            Sign in
          </Link>
        </>
      }
    >
      <form onSubmit={onSubmit}>
        <AuthFields mode="signup" error={error} pending={pending} submitLabel="Create account" />
      </form>
    </AuthLayout>
  );
}
