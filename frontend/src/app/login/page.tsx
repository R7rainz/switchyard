"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { AuthFields, AuthLayout } from "@/components/auth-form";
import { signIn } from "@/lib/auth-client";

export default function LoginPage() {
  const router = useRouter();
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

    setPending(false);
    if (result.error) {
      setError(result.error.message ?? "Sign in failed");
      return;
    }
    router.push("/workflows");
    // refresh() so any server component re-reads the new session cookie.
    router.refresh();
  }

  return (
    <AuthLayout
      title="Sign in"
      footer={
        <>
          No account?{" "}
          <Link href="/signup" className="text-bone hover:text-chalk">
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
