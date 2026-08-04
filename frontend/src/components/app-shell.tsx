"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import type { ReactNode } from "react";

import { signOut } from "@/lib/auth-client";
import { useWorkspace } from "@/lib/queries";

import { Button, Eyebrow, Skeleton, Wordmark, cx } from "./ui";

// Only routes that exist. A nav linking to a 404 is worse than a short nav;
// Runs and Settings arrive with their screens.
const links = [{ href: "/workflows", label: "Workflows" }];

/**
 * The chrome every signed-in page sits inside.
 *
 * Frosted rather than solid: the bar sits over cream and white bands, and a
 * hard fill would cut a line across both. One sticky bar, no sidebar.
 */
export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { workspace } = useWorkspace();

  return (
    <div className="flex min-h-screen flex-col bg-cream-wash">
      <header className="sticky top-0 z-10 border-b border-hairline bg-canvas-white/80 backdrop-blur-xl">
        <div className="mx-auto flex h-[62px] max-w-[1200px] items-center gap-8 px-6">
          <Link href="/workflows">
            <Wordmark />
          </Link>

          <nav className="flex items-center gap-1">
            {links.map((link) => {
              const active = pathname.startsWith(link.href);
              return (
                <Link
                  key={link.href}
                  href={link.href}
                  className={cx(
                    "rounded-lg px-3 py-2 text-body-sm",
                    active ? "bg-pearl text-ink" : "text-ash hover:text-ink",
                  )}
                >
                  {link.label}
                </Link>
              );
            })}
          </nav>

          <div className="ml-auto flex items-center gap-3">
            {workspace ? (
              <Eyebrow className="hidden sm:block">{workspace.name}</Eyebrow>
            ) : (
              <Skeleton className="hidden h-3 w-24 sm:block" />
            )}
            <Button
              variant="ghost"
              className="h-9"
              onClick={async () => {
                await signOut();
                router.push("/");
                router.refresh();
              }}
            >
              Sign out
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto w-full max-w-[1200px] flex-1 px-6 py-12">{children}</main>
    </div>
  );
}

/**
 * A page heading: 10px eyebrow above a 44px title at weight 400. Restraint at
 * that size is the signature — the system does not shout.
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
        <Eyebrow>{eyebrow}</Eyebrow>
        <h1 className="text-heading-sm text-ink sm:text-heading">{title}</h1>
      </div>
      {actions && <div className="flex items-center gap-3">{actions}</div>}
    </div>
  );
}
