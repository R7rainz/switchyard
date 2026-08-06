"use client";

import { Check, Copy, Trash2 } from "lucide-react";
import { useState } from "react";

import { PageHeader } from "@/components/app-shell";
import {
  Badge,
  Button,
  Card,
  Eyebrow,
  ErrorNote,
  Field,
  Input,
  Skeleton,
} from "@/components/ui";
import { AI_CREDENTIAL, AI_PROVIDERS, apiError, roles, type Role } from "@/lib/api";
import {
  useCreateInvite,
  useCredentials,
  useDeleteCredential,
  useInvites,
  useMembers,
  usePutCredential,
  useRemoveMember,
  useRevokeInvite,
  useSetRole,
  useWorkspace,
} from "@/lib/queries";
import { relativeTime } from "@/lib/time";

export default function SettingsPage() {
  const { workspace } = useWorkspace();

  return (
    <>
      <PageHeader eyebrow="Workspace" title="Settings" />
      <div className="flex flex-col gap-10">
        <Keys workspaceId={workspace?.id} />
        <Members workspaceId={workspace?.id} />
        <Invites workspaceId={workspace?.id} />
      </div>
    </>
  );
}

/**
 * Stored keys.
 *
 * There is no endpoint that returns a secret, so nothing here can show one —
 * a key is written, described, or replaced, and replacing is how you rotate.
 * The form says so rather than rendering a masked value that implies the
 * secret could be revealed.
 */
function Keys({ workspaceId }: { workspaceId: string | undefined }) {
  const { data: keys, isPending } = useCredentials(workspaceId);
  const put = usePutCredential(workspaceId);
  const remove = useDeleteCredential(workspaceId);

  const [provider, setProvider] = useState<string>(AI_CREDENTIAL.provider);
  const [name, setName] = useState<string>(AI_CREDENTIAL.name);
  const [secret, setSecret] = useState("");

  const hasAIKey = keys?.some(
    (key) => AI_PROVIDERS.includes(key.provider as (typeof AI_PROVIDERS)[number]) && key.name === AI_CREDENTIAL.name,
  );

  return (
    <section className="flex flex-col gap-4">
      <div>
        <Eyebrow>Credentials</Eyebrow>
        <p className="mt-2 max-w-xl text-body-sm leading-relaxed text-ash">
          Keys belong to the workspace, not to you — a key saved here is the key everyone&apos;s
          workflows run with. They are encrypted before storage and never returned; to change one,
          save over it.
        </p>
      </div>

      {!isPending && !hasAIKey && (
        <Card className="bg-canary-yellow/40">
          <p className="text-body-sm leading-relaxed text-ink">
            AI generation and AI nodes need a key stored as{" "}
            <code className="text-[13px]">provider/{AI_CREDENTIAL.name}</code>. Choose the
            provider in the node or generation dialog; without its matching key the run explains
            exactly what is missing.
          </p>
        </Card>
      )}

      <Card>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            put.mutate(
              { provider, name, secret },
              { onSuccess: () => setSecret("") },
            );
          }}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Provider">
              <select
                value={provider}
                onChange={(e) => setProvider(e.target.value)}
                className="h-12 rounded-xl border border-hairline bg-canvas-white px-4 text-body-sm text-ink focus:border-ink/25 focus:outline-none"
                required
              >
                {AI_PROVIDERS.map((option) => (
                  <option key={option} value={option}>
                    {option}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Name">
              <Input value={name} onChange={(e) => setName(e.target.value)} required />
            </Field>
          </div>
          <Field label="Secret" hint="Stored encrypted. It cannot be read back afterwards.">
            <Input
              type="password"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              placeholder="provider API key"
              required
            />
          </Field>
          {put.error && <ErrorNote>{apiError(put.error)}</ErrorNote>}
          <div className="flex justify-end">
            <Button type="submit" disabled={put.isPending || !secret}>
              {put.isPending ? "Saving…" : "Save key"}
            </Button>
          </div>
        </form>
      </Card>

      {isPending ? (
        <Skeleton className="h-14 w-full" />
      ) : keys && keys.length > 0 ? (
        <ul className="overflow-hidden rounded-xl border border-hairline bg-canvas-white">
          {keys.map((key, index) => (
            <li
              key={`${key.provider}/${key.name}`}
              className={`flex items-center gap-3 px-5 py-3.5 ${index > 0 ? "border-t border-hairline" : ""}`}
            >
              <span className="text-body-sm text-ink">
                {key.provider}/{key.name}
              </span>
              <Eyebrow className="ml-auto">updated {relativeTime(key.updatedAt)}</Eyebrow>
              <button
                onClick={() => remove.mutate({ provider: key.provider, name: key.name })}
                aria-label={`Delete ${key.provider}/${key.name}`}
                className="text-stone hover:text-phoenix-orange"
              >
                <Trash2 size={16} strokeWidth={1.75} />
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-body-sm text-ash">No keys stored yet.</p>
      )}
    </section>
  );
}

function Members({ workspaceId }: { workspaceId: string | undefined }) {
  const { data: list, isPending } = useMembers(workspaceId);
  const setRole = useSetRole(workspaceId);
  const remove = useRemoveMember(workspaceId);

  return (
    <section className="flex flex-col gap-4">
      <div>
        <Eyebrow>Members</Eyebrow>
        <p className="mt-2 max-w-xl text-body-sm leading-relaxed text-ash">
          Roles are strictly ordered. Nobody may grant a role above their own, act on a peer at the
          same level, or remove the last owner — the server enforces all three.
        </p>
      </div>

      {isPending ? (
        <Skeleton className="h-14 w-full" />
      ) : (
        <ul className="overflow-hidden rounded-xl border border-hairline bg-canvas-white">
          {list?.map((member, index) => (
            <li
              key={member.userId}
              className={`flex flex-wrap items-center gap-3 px-5 py-3.5 ${index > 0 ? "border-t border-hairline" : ""}`}
            >
              <span className="min-w-0 flex-1 truncate text-body-sm text-ink">{member.userId}</span>
              <select
                value={member.role}
                onChange={(e) => setRole.mutate({ userId: member.userId, role: e.target.value as Role })}
                className="h-9 rounded-lg border border-hairline bg-canvas-white px-2 text-caption text-ink"
              >
                {roles.map((role) => (
                  <option key={role} value={role}>
                    {role}
                  </option>
                ))}
              </select>
              <button
                onClick={() => remove.mutate(member.userId)}
                aria-label={`Remove ${member.userId}`}
                className="text-stone hover:text-phoenix-orange"
              >
                <Trash2 size={16} strokeWidth={1.75} />
              </button>
            </li>
          ))}
        </ul>
      )}
      {(setRole.error || remove.error) && (
        <ErrorNote>{apiError(setRole.error ?? remove.error)}</ErrorNote>
      )}
    </section>
  );
}

function Invites({ workspaceId }: { workspaceId: string | undefined }) {
  const { data: list } = useInvites(workspaceId);
  const create = useCreateInvite(workspaceId);
  const revoke = useRevokeInvite(workspaceId);

  const [role, setRole] = useState<Role>("MEMBER");
  const [copied, setCopied] = useState(false);

  return (
    <section className="flex flex-col gap-4">
      <div>
        <Eyebrow>Invites</Eyebrow>
        <p className="mt-2 max-w-xl text-body-sm leading-relaxed text-ash">
          A link is shown once. Only its hash is stored, so it cannot be displayed again — to
          re-share, revoke and issue a new one.
        </p>
      </div>

      <Card>
        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-2">
            <Eyebrow>Role</Eyebrow>
            <select
              value={role}
              onChange={(e) => setRole(e.target.value as Role)}
              className="h-10 rounded-lg border border-hairline bg-canvas-white px-3 text-body-sm text-ink"
            >
              {roles.map((one) => (
                <option key={one} value={one}>
                  {one}
                </option>
              ))}
            </select>
          </label>
          <Button
            onClick={() => {
              setCopied(false);
              create.mutate({ role });
            }}
            disabled={create.isPending}
          >
            {create.isPending ? "Creating…" : "Create invite link"}
          </Button>
        </div>

        {create.error && <ErrorNote>{apiError(create.error)}</ErrorNote>}

        {create.data && (
          // Shown once, and said so. The token is never retrievable again.
          <div className="mt-4 flex items-center gap-2 rounded-lg bg-mint-green/40 p-3">
            <code className="min-w-0 flex-1 truncate text-[12px] text-ink">
              {create.data.joinURL}
            </code>
            <Button
              variant="neutral"
              className="h-8 px-3"
              onClick={() => {
                navigator.clipboard.writeText(create.data!.joinURL);
                setCopied(true);
              }}
            >
              {copied ? <Check size={13} /> : <Copy size={13} />}
              {copied ? "Copied" : "Copy"}
            </Button>
          </div>
        )}
      </Card>

      {list && list.length > 0 && (
        <ul className="overflow-hidden rounded-xl border border-hairline bg-canvas-white">
          {list.map((invite, index) => (
            <li
              key={invite.id}
              className={`flex flex-wrap items-center gap-3 px-5 py-3.5 ${index > 0 ? "border-t border-hairline" : ""}`}
            >
              <Badge>{invite.role}</Badge>
              <span className="text-body-sm text-ash">
                {invite.email || "Anyone with the link"}
              </span>
              <Eyebrow className="ml-auto">
                {invite.useCount}
                {invite.maxUses > 0 ? `/${invite.maxUses}` : ""} used
              </Eyebrow>
              <button
                onClick={() => revoke.mutate(invite.id)}
                aria-label="Revoke invite"
                className="text-stone hover:text-phoenix-orange"
              >
                <Trash2 size={16} strokeWidth={1.75} />
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
