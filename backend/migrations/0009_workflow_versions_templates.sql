-- Workflow history is separate from execution snapshots: executions answer what
-- ran, while versions answer what a workflow looked like before an edit.
create table "workflow_version" (
  "id"          text primary key,
  "workspaceId" text not null references "workspace" ("id") on delete cascade,
  "workflowId"  text not null references "workflow" ("id") on delete cascade,
  "number"      integer not null,
  "name"        text not null,
  "description" text default '' not null,
  "graph"       jsonb not null,
  "createdBy"   text references "user" ("id") on delete set null,
  "createdAt"   timestamptz default CURRENT_TIMESTAMP not null,
  unique ("workflowId", "number")
);

create index "workflow_version_lookup_idx"
  on "workflow_version" ("workspaceId", "workflowId", "number");

create table "workflow_template" (
  "id"          text primary key,
  "workspaceId" text not null references "workspace" ("id") on delete cascade,
  "name"        text not null,
  "description" text default '' not null,
  "graph"       jsonb not null,
  "createdBy"   text references "user" ("id") on delete set null,
  "createdAt"   timestamptz default CURRENT_TIMESTAMP not null,
  constraint "workflow_template_name_not_blank" check (length(btrim("name")) > 0)
);

create index "workflow_template_workspace_idx"
  on "workflow_template" ("workspaceId", "name");
