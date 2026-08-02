-- Credentials: third-party secrets (provider API keys, OAuth token documents)
-- sealed with AES-256-GCM by internal/credential. Only ciphertext lands here;
-- the master keys live in the environment, never in the database.
--
-- "keyVersion" names which master key sealed the row, so a key rotation can
-- re-encrypt in batches while old and new rows coexist. Its index is what the
-- rotation scan reads.

create table "credentials" ("id" text not null primary key, "ownerId" text not null references "user" ("id") on delete cascade, "provider" text not null, "name" text not null, "ciphertext" bytea not null, "nonce" bytea not null, "keyVersion" integer not null, "createdAt" timestamptz default CURRENT_TIMESTAMP not null, "updatedAt" timestamptz default CURRENT_TIMESTAMP not null, unique ("ownerId", "provider", "name"));

-- No separate index on "ownerId": the unique constraint above already indexes
-- it as its leading column, which is what an owner-scoped lookup uses.
create index "credentials_keyVersion_idx" on "credentials" ("keyVersion");
