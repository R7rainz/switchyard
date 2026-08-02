-- Workspaces, membership, and invitations.
--
-- A workspace is what everything else belongs to: workflows, executions, and
-- credentials are owned by a workspace rather than by a user directly, so
-- access is a membership question rather than an ownership one.

create table "workspace" (
  "id"        text primary key,
  "name"      text not null,
  "slug"      text not null unique,
  "createdAt" timestamptz default CURRENT_TIMESTAMP not null,
  "updatedAt" timestamptz default CURRENT_TIMESTAMP not null
);

-- Role is text with a check rather than a Postgres enum: adding a value to an
-- enum needs DDL and locks, and the application already refuses anything it
-- does not recognise.
create table "workspace_member" (
  "id"          text primary key,
  "workspaceId" text not null references "workspace" ("id") on delete cascade,
  "userId"      text not null references "user" ("id") on delete cascade,
  "role"        text not null check ("role" in ('VIEWER', 'MEMBER', 'ADMIN', 'OWNER')),
  "createdAt"   timestamptz default CURRENT_TIMESTAMP not null,

  -- One standing per person per workspace. Two rows would make "what is their
  -- role" ambiguous, and the answer would depend on row order.
  unique ("workspaceId", "userId")
);

-- Listing a user's workspaces is the first query every request makes.
create index "workspace_member_userId_idx" on "workspace_member" ("userId");

create table "workspace_invite" (
  "id"          text primary key,
  "workspaceId" text not null references "workspace" ("id") on delete cascade,

  -- The SHA-256 of the token, never the token. An invite token is a bearer
  -- credential: storing it would let anyone who reads this table join every
  -- workspace with an invite outstanding.
  "tokenHash"   text not null unique,

  -- Null email means a shareable link rather than an invitation addressed to
  -- one person.
  "email"       text,
  "role"        text not null check ("role" in ('VIEWER', 'MEMBER', 'ADMIN', 'OWNER')),

  -- Null expiry never expires; null maxUses is unlimited.
  "expiresAt"   timestamptz,
  "maxUses"     integer,
  "useCount"    integer default 0 not null,
  "invitedBy"   text references "user" ("id") on delete set null,
  "createdAt"   timestamptz default CURRENT_TIMESTAMP not null,

  constraint "workspace_invite_uses_nonnegative" check ("useCount" >= 0),
  constraint "workspace_invite_maxUses_positive" check ("maxUses" is null or "maxUses" > 0)
);

create index "workspace_invite_workspaceId_idx" on "workspace_invite" ("workspaceId");
