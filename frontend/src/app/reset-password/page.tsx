"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";

import { AuthLayout } from "@/components/auth-form";
import { Button, ErrorNote, Field, Input } from "@/components/ui";
import { authClient } from "@/lib/auth-client";

export default function ResetPasswordPage() {
  return (
    <Suspense fallback={null}>
      <ResetPasswordForm />
    </Suspense>
  );
}

function ResetPasswordForm() {
  const params = useSearchParams();
  const token = params.get("token");
  const invalid = params.get("error");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [complete, setComplete] = useState(false);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    const form = new FormData(event.currentTarget);
    const password = String(form.get("password"));
    const confirmation = String(form.get("confirmation"));
    if (password !== confirmation) {
      setError("Passwords do not match");
      return;
    }

    setPending(true);
    const result = await authClient.resetPassword({ newPassword: password, token: token ?? "" });
    setPending(false);
    if (result.error) {
      setError(result.error.message ?? "Could not reset password");
      return;
    }
    setComplete(true);
  }

  return (
    <AuthLayout
      title={complete ? "Password updated" : "Set a new password"}
      subtitle={complete ? "Your account is ready to use again." : "Choose a password you have not used before."}
      footer={
        <>
          <Link href="/login" className="text-ink underline underline-offset-4">
            Back to sign in
          </Link>
        </>
      }
    >
      {complete ? (
        <Link href="/login" className="text-body-sm text-ink underline underline-offset-4">
          Sign in with your new password
        </Link>
      ) : !token || invalid ? (
        <div className="flex flex-col gap-5">
          <ErrorNote>This reset link is invalid or has expired.</ErrorNote>
          <Link href="/forgot-password" className="text-body-sm text-ink underline underline-offset-4">
            Request a new link
          </Link>
        </div>
      ) : (
        <form onSubmit={onSubmit} className="flex flex-col gap-5">
          <Field label="New password">
            <Input
              name="password"
              type="password"
              required
              minLength={8}
              autoComplete="new-password"
              placeholder="At least 8 characters"
            />
          </Field>

          <Field label="Confirm password">
            <Input
              name="confirmation"
              type="password"
              required
              minLength={8}
              autoComplete="new-password"
              placeholder="Enter it again"
            />
          </Field>

          {error && <ErrorNote>{error}</ErrorNote>}

          <Button type="submit" disabled={pending} className="h-12 w-full">
            {pending ? "Updating…" : "Update password"}
          </Button>
        </form>
      )}
    </AuthLayout>
  );
}
