-- Move credentials from a user to a workspace.
--
-- 0002 predates workspaces and scoped each credential to the user who saved
-- it. Now that a workspace owns workflows and executions, it has to own the
-- keys they run with too: otherwise an admin cannot manage the workspace's
-- credentials, and a workflow shared with the workspace fails for everyone
-- except the person whose key it uses.
--
-- The table is recreated rather than altered because there is nothing to
-- migrate. No SQL store was ever written for it, so nothing has inserted a
-- row, and there is no user-to-workspace mapping that would make an automatic
-- rewrite correct anyway.
--
-- The guard makes that assumption explicit: if any row exists, this refuses to
-- run rather than deleting data. Whoever hits it can decide which workspace
-- those credentials belong to, which is a judgement no migration can make.
do $$
begin
  if exists (select 1 from "credentials") then
    raise exception
      'credentials has rows; 0004 will not guess which workspace they belong to. Move them by hand, then re-run.';
  end if;
end $$;

drop table "credentials";

create table "credentials" (
  "id"          text primary key,
  "workspaceId" text not null references "workspace" ("id") on delete cascade,
  "provider"    text not null,
  "name"        text not null,
  "ciphertext"  bytea not null,
  "nonce"       bytea not null,
  "keyVersion"  integer not null,
  "createdAt"   timestamptz default CURRENT_TIMESTAMP not null,
  "updatedAt"   timestamptz default CURRENT_TIMESTAMP not null,

  -- One credential per provider and name within a workspace. This also indexes
  -- workspaceId as its leading column, which is what every scoped lookup uses,
  -- so no separate index earns its place.
  unique ("workspaceId", "provider", "name")
);

-- Deleting a workspace takes its credentials with it, which the cascade above
-- handles. Deleting a *user* must not: the keys belong to the workspace, and
-- the member who happened to add them leaving is not a reason to break every
-- workflow that uses them. That is why there is no longer a reference to
-- "user" here at all.
