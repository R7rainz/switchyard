"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import type { ReactNode } from "react";

import { signOut, useSession } from "@/lib/auth-client";
import { useWorkspace } from "@/lib/queries";

import { Mono, Skeleton, cx } from "./ui";

// Only what exists. A nav that links to a 404 is worse than a shorter nav;
// Runs and Settings arrive with their screens.
const links = [{ href: "/workflows", label: "Workflows" }];

/**
 * The chrome every signed-in page sits inside.
 *
 * One sticky bar, no sidebar, no mega-menu — the surface stays uncluttered and
 * the work is the only object in the room.
 */
export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { data: session } = useSession();
  const { workspace } = useWorkspace();

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-10 border-b border-carbon-lift bg-obsidian-canvas/95 backdrop-blur-sm">
        <div className="mx-auto flex h-16 max-w-[1200px] items-center gap-8 px-6">
          <Link href="/workflows" className="flex items-center gap-2">
            <Mono className="tracking-[0.08em] text-bone">Switchyard</Mono>
          </Link>

          <nav className="flex items-center gap-1">
            {links.map((link) => {
              const active = pathname.startsWith(link.href);
              return (
                <Link
                  key={link.href}
                  href={link.href}
                  // The active tab is stated in weight of colour, not a pill or
                  // an underline — the chrome stays quiet.
                  className={cx(
                    "rounded-sm px-3 py-1.5 text-body-sm",
                    active ? "text-bone" : "text-warm-granite hover:text-pale-stone",
                  )}
                >
                  {link.label}
                </Link>
              );
            })}
          </nav>

          <div className="ml-auto flex items-center gap-4">
            {workspace ? (
              <Mono className="hidden text-pale-stone sm:block">{workspace.name}</Mono>
            ) : (
              <Skeleton className="hidden h-3 w-24 sm:block" />
            )}
            <span aria-hidden className="hidden h-4 w-px bg-carbon-lift sm:block" />
            <button
              onClick={async () => {
                await signOut();
                router.push("/");
                router.refresh();
              }}
              className="text-body-sm text-warm-granite hover:text-bone"
            >
              {session?.user.name ? `Sign out` : "Sign out"}
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto w-full max-w-[1200px] flex-1 px-6 py-10">{children}</main>
    </div>
  );
}

/**
 * A page heading: mono eyebrow above a 36px title, with actions on the right.
 * The eyebrow is what tells you this is an instrument and not a page.
 */
export function PageHeader({
  eyebrow,
  title,
  actions,
}: {
  eyebrow: string;
  title: string;
  actions?: ReactNode;
}) {
  return (
    <div className="mb-10 flex flex-wrap items-end justify-between gap-4">
      <div className="flex flex-col gap-3">
        <Mono>{eyebrow}</Mono>
        <h1 className="text-heading text-bone">{title}</h1>
      </div>
      {actions && <div className="flex items-center gap-3">{actions}</div>}
    </div>
  );
}
