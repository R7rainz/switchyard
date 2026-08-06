alter table "execution"
  add column "retryOf" text references "execution" ("id") on delete set null,
  add column "idempotencyKey" text;

create unique index "execution_workspaceId_idempotencyKey_idx"
  on "execution" ("workspaceId", "idempotencyKey")
  where "idempotencyKey" is not null;
