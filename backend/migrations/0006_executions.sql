-- Executions: one run of one workflow.
--
-- This is where determinism is actually enforced. The graph is copied onto the
-- execution row when the run starts, so what happened is a fact about this row
-- rather than a lookup into a workflow that may since have been edited. It is
-- also why there is no workflow_version table: the snapshot answers "what ran",
-- which is the question versioning would have been for.

create table "execution" (
  "id"          text primary key,
  "workspaceId" text not null references "workspace" ("id") on delete cascade,

  -- The workflow may be deleted while its history is still worth keeping, so
  -- this goes null rather than taking the executions with it. The snapshot
  -- below means the record is still readable afterwards.
  "workflowId"  text references "workflow" ("id") on delete set null,

  -- What ran. Not a reference to what the workflow says today.
  "graph"       jsonb not null,

  "status"      text not null check ("status" in
                  ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELLED')),

  -- How it started: "manual", "webhook", "schedule". Kept as text because the
  -- set grows with trigger node types and the application already refuses what
  -- it does not recognise.
  "trigger"     text not null,

  -- The payload the trigger carried, and what the run produced.
  "input"       jsonb,
  "error"       text,

  "startedBy"   text references "user" ("id") on delete set null,
  "createdAt"   timestamptz default CURRENT_TIMESTAMP not null,
  "startedAt"   timestamptz,
  "finishedAt"  timestamptz
);

-- The execution list is per workspace, newest first, and often filtered to one
-- workflow.
create index "execution_workspace_created_idx"
  on "execution" ("workspaceId", "createdAt" desc);
create index "execution_workflow_created_idx"
  on "execution" ("workflowId", "createdAt" desc);

-- Reclaiming runs abandoned by a crashed process scans for this.
create index "execution_status_idx" on "execution" ("status")
  where "status" in ('PENDING', 'RUNNING');

-- One row per node per run. This is the explainability requirement made
-- concrete: which node ran, in what order, how long it took, what came back,
-- and which one failed.
create table "execution_node" (
  "id"          text primary key,
  "executionId" text not null references "execution" ("id") on delete cascade,
  "nodeId"      text not null,

  -- SKIPPED is a real outcome, not an absence: a node on the branch that was
  -- not taken has to be distinguishable in the viewer from one still pending.
  "status"      text not null check ("status" in
                  ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELLED', 'SKIPPED')),

  "output"      jsonb,
  "error"       text,
  "startedAt"   timestamptz,
  "finishedAt"  timestamptz,

  -- One result per node per run. The engine writes a row when a node starts and
  -- updates it when the node finishes, so this upserts.
  unique ("executionId", "nodeId")
);

create index "execution_node_execution_idx" on "execution_node" ("executionId");
