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

/**
 * Enter the app after signing in or up.
 *
 * A full navigation rather than router.push, and that is the fix for "I have to
 * log in twice".
 *
 * useSession is one shared store, not per-component state. A visitor who has
 * already been on a page that read it — the landing page does — is carrying
 * `data: null, isPending: false`, and a client-side navigation to the login
 * form does not reset it. So after a successful sign-in the guard on the next
 * page read that *stale* null, decided nobody was signed in, and redirected
 * straight back to the form: the first attempt succeeded, the session was real,
 * and the UI threw it away. Opening /login directly happened to reset the
 * store, which is why it never reproduced that way and why a second attempt
 * always worked.
 *
 * Awaiting getSession() first does not fix it — that call does not notify the
 * store useSession subscribes to. A document navigation resets every store
 * there is, and has the server re-read the session cookie on the way, which is
 * what the router.refresh() beside it was reaching for anyway.
 *
 * replace rather than assign, so the back button after signing in does not go
 * to the login form of a session that already exists.
 */
export function enterApp() {
  window.location.replace("/workflows");
}

/**
 * Leave the app after signing out.
 *
 * The mirror of enterApp, and the same race in the other direction: signing out
 * empties the session store, the guard on the page you are still standing on
 * reacts by redirecting to /login, and that beats the push to the landing page.
 * Signing out then dumped you on a login form rather than the marketing page.
 */
export function leaveApp() {
  window.location.replace("/");
}
