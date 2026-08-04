"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Button, ErrorNote, Splash, Wordmark } from "@/components/ui";
import { apiError, invites } from "@/lib/api";
import { enterApp, useSession } from "@/lib/auth-client";

/**
 * Accepting an invite.
 *
 * The backend has always handed out {appURL}/invite/{token} and there was no
 * page here to answer it, so every link 404'd — the token was fine, the route
 * did not exist.
 *
 * Accepting needs a signed-in user, because an invite grants membership to a
 * person rather than to a browser. Someone opening the link in a browser they
 * have never signed into is the normal case, not the exception, so the token is
 * carried through sign-up and back rather than lost on the way.
 */
export default function InvitePage() {
  const { token } = useParams<{ token: string }>();
  const { data: session, isPending } = useSession();

  const [error, setError] = useState<string | null>(null);
  // Accepting twice is not an error on the server, but firing it twice from one
  // page load is still noise — and React mounts effects twice in development.
  const attempted = useRef(false);

  useEffect(() => {
    if (isPending || !session || attempted.current) return;
    attempted.current = true;

    invites
      .accept(token)
      // Straight into the workspace they just joined. A full navigation, for
      // the same reason signing in uses one: the session store is shared and
      // the guard on the next page reads it synchronously.
      .then(() => enterApp())
      .catch((cause) => setError(apiError(cause)));
  }, [isPending, session, token]);

  if (isPending) return <Splash />;

  if (!session) {
    // The token rides in the return path, so signing up does not lose it.
    const next = `/invite/${token}`;
    return (
      <main className="flex min-h-screen flex-col items-center justify-center gap-8 px-6">
        <Wordmark />
        <div className="w-full max-w-sm rounded-xl border border-hairline p-8 text-center">
          <h1 className="text-heading-sm text-ink">You have been invited</h1>
          <p className="mt-3 text-body-sm leading-relaxed text-ash">
            Sign in or create an account to join this workspace. The invite is held until you do.
          </p>
          <div className="mt-8 flex flex-col gap-3">
            <Link href={`/signup?next=${encodeURIComponent(next)}`}>
              <Button className="h-12 w-full">Create an account</Button>
            </Link>
            <Link href={`/login?next=${encodeURIComponent(next)}`}>
              <Button variant="neutral" className="h-12 w-full">
                Sign in
              </Button>
            </Link>
          </div>
        </div>
      </main>
    );
  }

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 px-6">
      <Wordmark />
      {error ? (
        <div className="w-full max-w-sm rounded-xl border border-hairline p-8">
          <ErrorNote>{error}</ErrorNote>
          <p className="mt-4 text-body-sm leading-relaxed text-ash">
            An invite can expire, run out of uses, or be revoked. Ask whoever sent it for a new
            link — a used one cannot be re-shared, only reissued.
          </p>
          <Link href="/workflows">
            <Button variant="neutral" className="mt-6 h-11 w-full">
              Go to your workspaces
            </Button>
          </Link>
        </div>
      ) : (
        <p className="text-body-sm text-ash">Joining…</p>
      )}
    </main>
  );
}
