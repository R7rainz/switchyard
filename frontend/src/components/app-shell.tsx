"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Activity, BookOpen, LogOut, Settings, Workflow } from "lucide-react";
import type { ReactNode } from "react";

import { leaveApp, signOut } from "@/lib/auth-client";
import { useWorkspace } from "@/lib/queries";

import { Button, Eyebrow, Skeleton, Wordmark, cx } from "./ui";

// Only routes that exist. A nav linking to a 404 is worse than a short nav.
const links = [
  { href: "/workflows", label: "Workflows", icon: Workflow },
  { href: "/runs", label: "Runs", icon: Activity },
  { href: "/docs", label: "Docs", icon: BookOpen },
  { href: "/settings", label: "Settings", icon: Settings },
];

/**
 * The chrome every signed-in page sits inside.
 *
 * Frosted rather than solid: the bar sits over cream and white bands, and a
 * hard fill would cut a line across both. One sticky bar, no sidebar.
 */
export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { workspace, workspaces, selectWorkspace } = useWorkspace();

  return (
    <div className="flex min-h-screen flex-col bg-cream-wash">
      <header className="sticky top-0 z-10 border-b border-hairline bg-canvas-white/80 backdrop-blur-xl">
        <div className="mx-auto flex h-[62px] max-w-[1200px] items-center gap-4 px-4 sm:px-6 md:gap-8">
          <Link href="/workflows">
            <Wordmark className="[&>span:last-child]:hidden sm:[&>span:last-child]:inline" />
          </Link>

          <nav className="hidden items-center gap-1 md:flex">
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

          <div className="ml-auto flex min-w-0 items-center gap-2 sm:gap-3">
            {workspace ? (
              workspaces.length > 1 ? (
                <select
                  aria-label="Current workspace"
                  value={workspace.id}
                  onChange={(event) => selectWorkspace(event.target.value)}
                  className="h-9 max-w-36 rounded-lg border border-hairline bg-canvas-white px-2 text-body-sm text-ink focus:border-ink/25 focus:outline-none sm:max-w-44"
                >
                  {workspaces.map((one) => <option key={one.id} value={one.id}>{one.name}</option>)}
                </select>
              ) : <Eyebrow className="hidden sm:block">{workspace.name}</Eyebrow>
            ) : (
              <Skeleton className="hidden h-3 w-24 sm:block" />
            )}
            <Button
              variant="ghost"
              className="size-9 px-0 sm:w-auto sm:px-4"
              aria-label="Sign out"
              onClick={async () => {
                await signOut();
                leaveApp();
              }}
            >
              <LogOut size={16} strokeWidth={1.75} className="sm:hidden" />
              <span className="hidden sm:inline">Sign out</span>
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto w-full max-w-[1200px] flex-1 px-4 py-8 pb-28 sm:px-6 sm:py-12 sm:pb-28 md:pb-12">{children}</main>

      <nav
        aria-label="Application"
        className="fixed inset-x-3 bottom-3 z-20 grid grid-cols-4 rounded-xl border border-hairline bg-canvas-white/95 p-1.5 shadow-raised backdrop-blur-xl md:hidden"
      >
        {links.map((link) => {
          const active = pathname.startsWith(link.href);
          const Icon = link.icon;
          return (
            <Link
              key={link.href}
              href={link.href}
              className={cx(
                "flex min-w-0 flex-col items-center gap-1 rounded-lg px-1 py-2 text-[10px]",
                active ? "bg-ink text-canvas-white" : "text-ash",
              )}
            >
              <Icon size={17} strokeWidth={1.75} />
              <span className="truncate">{link.label}</span>
            </Link>
          );
        })}
      </nav>
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
    <div className="mb-8 flex flex-wrap items-end justify-between gap-5 sm:mb-10">
      <div className="flex flex-col gap-3">
        <Eyebrow>{eyebrow}</Eyebrow>
        <h1 className="text-heading-sm text-ink sm:text-heading">{title}</h1>
      </div>
      {actions && <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto sm:gap-3">{actions}</div>}
    </div>
  );
}
