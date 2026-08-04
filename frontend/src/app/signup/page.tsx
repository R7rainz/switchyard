"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { AuthFields, AuthLayout } from "@/components/auth-form";
import { signUp } from "@/lib/auth-client";

export default function SignupPage() {
  const router = useRouter();
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

    setPending(false);
    if (result.error) {
      setError(result.error.message ?? "Sign up failed");
      return;
    }
    // Better Auth signs the new user in, and listing workspaces creates their
    // personal one, so the dashboard is ready by the time they arrive.
    router.push("/workflows");
    router.refresh();
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
