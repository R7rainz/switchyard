import { createAuthClient } from "better-auth/react";

export const authClient = createAuthClient();

export const { signIn, signUp, signOut, useSession } = authClient;

/**
 * Mints a JWT for the current session, for calls to the Go backend.
 *
 * Short-lived by design (15m) — fetch one per call rather than caching it.
 * Returns null when there is no session.
 */
export async function getToken(): Promise<string | null> {
  const res = await fetch("/api/auth/token", { credentials: "include" });
  if (!res.ok) return null;
  const { token } = await res.json();
  return token ?? null;
}
