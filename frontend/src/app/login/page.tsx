"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";

import { AuthFields, AuthLayout } from "@/components/auth-form";
import { enterApp, signIn } from "@/lib/auth-client";

export default function LoginPage() {
  // Suspense because useSearchParams opts the page into client rendering, and
  // Next requires the boundary for it during prerender.
  return (
    <Suspense fallback={null}>
      <LoginPageForm />
    </Suspense>
  );
}

function LoginPageForm() {
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
    enterApp(next ?? undefined);
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
