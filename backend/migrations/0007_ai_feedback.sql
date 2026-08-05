-- Feedback is a separate, explicit action from generation. The generator
-- still creates no row; this table is written only after the user opts in.
create table "ai_feedback" (
  "id"                  text primary key,
  "workspaceId"         text not null references "workspace" ("id") on delete cascade,
  "userId"              text not null references "user" ("id") on delete cascade,
  "prompt"              text not null,
  "outcome"             text not null check ("outcome" in ('accepted', 'rejected')),
  "generatedName"       text not null,
  "generatedDescription" text not null,
  "generatedGraph"      jsonb not null,
  "finalGraph"          jsonb,
  "createdAt"           timestamptz not null default CURRENT_TIMESTAMP
);

create index "ai_feedback_workspaceId_idx" on "ai_feedback" ("workspaceId", "createdAt");
