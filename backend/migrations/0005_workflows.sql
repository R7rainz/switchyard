-- Workflows: the saved graph a user draws in the builder.
--
-- A workflow is data. Nothing here records a run; executions land in their own
-- table, and each one keeps the graph it ran so a later edit cannot change what
-- an old execution appears to have done.
--
-- There is no version table yet. Determinism is what would demand one, and
-- pinning the graph on the execution row satisfies that completely; history and
-- rollback are a product feature, and an additive migration when they are
-- wanted.

create table "workflow" (
  "id"          text primary key,
  "workspaceId" text not null references "workspace" ("id") on delete cascade,
  "name"        text not null,
  "description" text default '' not null,

  -- jsonb, not normalized node and edge tables. Nothing ever reads one node:
  -- the builder sends the whole graph, the engine walks the whole graph, and a
  -- diff compares whole graphs. Splitting it would cost a join and N inserts
  -- per autosave and buy back a query nobody makes.
  --
  -- jsonb rather than json so equality and containment work if a later feature
  -- wants them; the reformatting it does on the way in is invisible here,
  -- because node positions are the only ordering anyone can see and they are
  -- carried inside the objects.
  "graph"       jsonb not null,

  -- Null once the creator's account is deleted; the workflow belongs to the
  -- workspace, so it outlives them.
  "createdBy"   text references "user" ("id") on delete set null,
  "createdAt"   timestamptz default CURRENT_TIMESTAMP not null,
  "updatedAt"   timestamptz default CURRENT_TIMESTAMP not null,

  constraint "workflow_name_not_blank" check (length(btrim("name")) > 0)
);

-- Listing a workspace's workflows is the dashboard's first query, and the
-- workspace is in the WHERE clause of every other one as well.
create index "workflow_workspaceId_idx" on "workflow" ("workspaceId");
