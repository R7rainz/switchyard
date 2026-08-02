"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { API_URL } from "@/lib/api";
import { getToken, signOut, useSession } from "@/lib/auth-client";

export default function Home() {
  const router = useRouter();
  const { data: session, isPending } = useSession();

  if (isPending) return <main className="p-6">Loading…</main>;

  if (!session) {
    return (
      <main className="mx-auto flex min-h-screen max-w-sm flex-col justify-center gap-4 p-6">
        <h1 className="text-2xl font-semibold">Switchyard</h1>
        <p className="text-sm">AI drafts the route. You throw the switches.</p>
        <div className="flex gap-3">
          <Link href="/login" className="rounded bg-black px-3 py-2 text-white">
            Sign in
          </Link>
          <Link href="/signup" className="rounded border border-gray-400 px-3 py-2">
            Sign up
          </Link>
        </div>
      </main>
    );
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-2xl flex-col gap-8 p-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">Switchyard</h1>
          <p className="text-sm">
            Signed in as {session.user.name} ({session.user.email})
          </p>
        </div>
        <button
          onClick={async () => {
            await signOut();
            router.refresh();
          }}
          className="rounded border border-gray-400 px-3 py-2 text-sm"
        >
          Sign out
        </button>
      </header>

      <BackendCheck />
    </main>
  );
}

/**
 * Calls the Go backend with a freshly minted JWT. This is the only thing on
 * the page that proves the whole chain — Better Auth session, JWKS, Ed25519
 * verification — so both the response and the failure are shown verbatim.
 */
function BackendCheck() {
  const [result, setResult] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function call() {
    setPending(true);
    setResult(null);
    setError(null);
    try {
      const token = await getToken();
      if (!token) throw new Error("no token — /api/auth/token returned nothing");

      const res = await fetch(`${API_URL}/api/me`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      const body = await res.text();
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}: ${body}`);
      setResult(body);
    } catch (cause) {
      // A CORS rejection or a dead backend both arrive as an opaque
      // "Failed to fetch", so name the URL that was tried.
      setError(`${cause instanceof Error ? cause.message : String(cause)} (GET ${API_URL}/api/me)`);
    } finally {
      setPending(false);
    }
  }

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-lg font-medium">Backend check</h2>
      <button
        onClick={call}
        disabled={pending}
        className="self-start rounded bg-black px-3 py-2 text-sm text-white disabled:opacity-50"
      >
        {pending ? "Calling…" : "GET /api/me"}
      </button>

      {result && (
        <pre className="overflow-x-auto rounded border border-green-600 p-3 text-sm">{result}</pre>
      )}
      {error && (
        <pre
          role="alert"
          className="overflow-x-auto rounded border border-red-500 p-3 text-sm text-red-600"
        >
          {error}
        </pre>
      )}
    </section>
  );
}
