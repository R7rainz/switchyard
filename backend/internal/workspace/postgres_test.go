package workspace

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/database"
	"github.com/R7rainz/switchyard/backend/migrations"
)

// postgresStore connects to SWITCHYARD_TEST_DATABASE_URL, migrates it, and
// clears the workspace tables. It skips when the variable is unset, so the
// normal `go test ./...` stays offline.
//
// It drops and recreates the public schema, so never point it at a database
// anyone cares about.
func postgresStore(t *testing.T) (*PostgresStore, *pgxpool.Pool) {
	t.Helper()

	url := os.Getenv("SWITCHYARD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set SWITCHYARD_TEST_DATABASE_URL to run the Postgres store tests")
	}

	ctx := context.Background()
	// One connection, so the advisory lock below is held on it for the whole
	// test rather than on whichever pooled session happened to take it.
	pool, err := database.Connect(ctx, database.Options{URL: url, MaxConns: 1, ConnectTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(pool.Close)

	// go test runs packages in parallel, and every one of these tests resets
	// the same schema. Without this they tear the database out from under each
	// other. Same trick the migration runner uses, different key; Postgres
	// drops the lock when the pool closes.
	if _, err := pool.Exec(ctx, "select pg_advisory_lock($1)", database.TestSchemaLock); err != nil {
		t.Fatalf("taking the test schema lock: %v", err)
	}

	if _, err := pool.Exec(ctx, "drop schema public cascade; create schema public"); err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
	if _, err := database.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewPostgresStore(pool), pool
}

// insertUser creates a row in the Better Auth user table, which membership and
// invites both reference.
func insertUser(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`insert into "user" ("id", "name", "email", "emailVerified") values ($1, $1, $1 || '@test.local', false)`,
		id)
	if err != nil {
		t.Fatalf("inserting user %s: %v", id, err)
	}
}

// serviceOn wires the real service over Postgres, so these tests exercise the
// rules and the SQL together rather than the queries in isolation.
func serviceOn(store *PostgresStore) *Service {
	return NewService(store)
}

func TestPostgresWorkspaceAndMemberRoundTrip(t *testing.T) {
	store, pool := postgresStore(t)
	insertUser(t, pool, "owner-1")
	ctx := context.Background()

	created, err := serviceOn(store).Create(ctx, "owner-1", "Switchyard", "switchyard")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	loaded, err := store.GetWorkspace(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if loaded.Name != "Switchyard" || loaded.Slug != "switchyard" {
		t.Errorf("loaded = %+v, want the created values", loaded)
	}

	// Create must have installed the owner in the same call.
	member, err := store.GetMember(ctx, created.ID, "owner-1")
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if member.Role != auth.RoleOwner {
		t.Errorf("role = %s, want OWNER", member.Role)
	}
}

func TestPostgresMissingRowsAreErrNotFound(t *testing.T) {
	// The service turns ErrNotFound into ErrNotMember and 404s, so a store that
	// returned a raw pgx error instead would surface as a 500.
	store, _ := postgresStore(t)
	ctx := context.Background()

	if _, err := store.GetWorkspace(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetWorkspace = %v, want ErrNotFound", err)
	}
	if _, err := store.GetMember(ctx, "nope", "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetMember = %v, want ErrNotFound", err)
	}
	if _, err := store.GetInviteByTokenHash(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetInviteByTokenHash = %v, want ErrNotFound", err)
	}
	if err := store.DeleteMember(ctx, "nope", "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteMember = %v, want ErrNotFound", err)
	}
	if err := store.DeleteInvite(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteInvite = %v, want ErrNotFound", err)
	}
}

func TestPostgresPutMemberUpdatesRatherThanDuplicates(t *testing.T) {
	// The unique constraint would reject a second insert outright, so without
	// the ON CONFLICT clause every role change would fail.
	store, pool := postgresStore(t)
	insertUser(t, pool, "owner-1")
	insertUser(t, pool, "member-1")
	ctx := context.Background()

	created, err := serviceOn(store).Create(ctx, "owner-1", "Test", "test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	first := Member{WorkspaceID: created.ID, UserID: "member-1", Role: auth.RoleViewer, CreatedAt: time.Now()}
	if err := store.PutMember(ctx, first); err != nil {
		t.Fatalf("PutMember: %v", err)
	}
	joined, err := store.GetMember(ctx, created.ID, "member-1")
	if err != nil {
		t.Fatal(err)
	}

	promoted := first
	promoted.Role = auth.RoleAdmin
	promoted.CreatedAt = time.Now().Add(time.Hour)
	if err := store.PutMember(ctx, promoted); err != nil {
		t.Fatalf("PutMember on conflict: %v", err)
	}

	members, err := store.ListMembers(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("workspace has %d members, want 2 — the upsert duplicated a row", len(members))
	}

	after, err := store.GetMember(ctx, created.ID, "member-1")
	if err != nil {
		t.Fatal(err)
	}
	if after.Role != auth.RoleAdmin {
		t.Errorf("role = %s, want ADMIN", after.Role)
	}
	// A promotion is not a rejoin, so the original join time survives.
	if !after.CreatedAt.Equal(joined.CreatedAt) {
		t.Errorf("createdAt moved from %v to %v on a role change", joined.CreatedAt, after.CreatedAt)
	}
}

func TestPostgresListWorkspacesForUserIsScoped(t *testing.T) {
	store, pool := postgresStore(t)
	insertUser(t, pool, "owner-1")
	insertUser(t, pool, "owner-2")
	ctx := context.Background()
	service := serviceOn(store)

	mine, err := service.Create(ctx, "owner-1", "Mine", "mine")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, "owner-2", "Theirs", "theirs"); err != nil {
		t.Fatal(err)
	}

	found, err := store.ListWorkspacesForUser(ctx, "owner-1")
	if err != nil {
		t.Fatalf("ListWorkspacesForUser: %v", err)
	}
	if len(found) != 1 || found[0].ID != mine.ID {
		t.Fatalf("owner-1 sees %d workspaces %v, want only their own", len(found), found)
	}
}

// The domain uses zero values where the schema uses NULL, and this is where a
// mistranslation would show up: a link invite read back as addressed, or an
// unlimited one read back as exhausted.
func TestPostgresInviteNullablesRoundTrip(t *testing.T) {
	store, pool := postgresStore(t)
	insertUser(t, pool, "owner-1")
	ctx := context.Background()
	service := serviceOn(store)

	created, err := service.Create(ctx, "owner-1", "Test", "test")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("shareable link with no expiry and no cap", func(t *testing.T) {
		invite, token, err := service.Invite(ctx, created.ID, "owner-1", InviteRequest{
			Role: auth.RoleViewer,
			TTL:  -1,
		})
		if err != nil {
			t.Fatalf("Invite: %v", err)
		}

		back, err := store.GetInviteByTokenHash(ctx, HashToken(token))
		if err != nil {
			t.Fatalf("GetInviteByTokenHash: %v", err)
		}
		if !back.IsLink() {
			t.Errorf("email = %q, want empty so IsLink holds", back.Email)
		}
		if !back.ExpiresAt.IsZero() {
			t.Errorf("expiresAt = %v, want zero", back.ExpiresAt)
		}
		if back.MaxUses != 0 {
			t.Errorf("maxUses = %d, want 0 meaning unlimited", back.MaxUses)
		}
		if back.Exhausted() {
			t.Error("an uncapped invite reported itself exhausted")
		}
		if back.ID != invite.ID || back.Role != auth.RoleViewer {
			t.Errorf("round trip lost fields: %+v", back)
		}
	})

	t.Run("addressed invite with an expiry", func(t *testing.T) {
		_, token, err := service.Invite(ctx, created.ID, "owner-1", InviteRequest{
			Email: "Someone@Example.test",
			Role:  auth.RoleMember,
			TTL:   time.Hour,
		})
		if err != nil {
			t.Fatalf("Invite: %v", err)
		}

		back, err := store.GetInviteByTokenHash(ctx, HashToken(token))
		if err != nil {
			t.Fatal(err)
		}
		if back.IsLink() {
			t.Error("an addressed invite came back as a link")
		}
		if back.Email != "someone@example.test" {
			t.Errorf("email = %q, want it lowercased", back.Email)
		}
		if back.ExpiresAt.IsZero() {
			t.Error("expiresAt came back zero")
		}
		if back.MaxUses != 1 {
			t.Errorf("maxUses = %d, want 1 — addressed invites are single use", back.MaxUses)
		}
	})
}

func TestPostgresAcceptFlow(t *testing.T) {
	// The whole point of the store: an invite created in one call is redeemable
	// in another, through the database rather than a map.
	store, pool := postgresStore(t)
	insertUser(t, pool, "owner-1")
	insertUser(t, pool, "joiner-1")
	insertUser(t, pool, "joiner-2")
	ctx := context.Background()
	service := serviceOn(store)

	created, err := service.Create(ctx, "owner-1", "Test", "test")
	if err != nil {
		t.Fatal(err)
	}

	_, token, err := service.Invite(ctx, created.ID, "owner-1", InviteRequest{Role: auth.RoleMember, MaxUses: 2})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	if _, err := service.Accept(ctx, token, "joiner-1"); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	role, err := service.RoleOf(ctx, created.ID, "joiner-1")
	if err != nil || role != auth.RoleMember {
		t.Fatalf("RoleOf = %s, %v; want MEMBER", role, err)
	}

	// The use count has to persist, or a capped link would never run out.
	if _, err := service.Accept(ctx, token, "joiner-2"); err != nil {
		t.Fatalf("second Accept: %v", err)
	}
	insertUser(t, pool, "joiner-3")
	if _, err := service.Accept(ctx, token, "joiner-3"); !errors.Is(err, ErrInviteExhausted) {
		t.Errorf("third Accept = %v, want ErrInviteExhausted", err)
	}
}

func TestPostgresAddressedInviteIsConsumed(t *testing.T) {
	store, pool := postgresStore(t)
	insertUser(t, pool, "owner-1")
	insertUser(t, pool, "joiner-1")
	ctx := context.Background()
	service := serviceOn(store)

	created, err := service.Create(ctx, "owner-1", "Test", "test")
	if err != nil {
		t.Fatal(err)
	}

	_, token, err := service.Invite(ctx, created.ID, "owner-1", InviteRequest{
		Email: "one@example.test", Role: auth.RoleMember,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Accept(ctx, token, "joiner-1"); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	// The row must be gone, so a forwarded email cannot admit a second person.
	if _, err := store.GetInviteByTokenHash(ctx, HashToken(token)); !errors.Is(err, ErrNotFound) {
		t.Errorf("a consumed invite is still in the database: %v", err)
	}
}

func TestPostgresCascadeOnWorkspaceDelete(t *testing.T) {
	// Members and invites hang off the workspace, and the schema says they go
	// with it. Worth proving, since nothing in Go enforces that.
	store, pool := postgresStore(t)
	insertUser(t, pool, "owner-1")
	ctx := context.Background()
	service := serviceOn(store)

	created, err := service.Create(ctx, "owner-1", "Test", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Invite(ctx, created.ID, "owner-1", InviteRequest{Role: auth.RoleViewer}); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `delete from "workspace" where "id" = $1`, created.ID); err != nil {
		t.Fatalf("deleting workspace: %v", err)
	}

	if _, err := store.GetMember(ctx, created.ID, "owner-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("membership outlived its workspace: %v", err)
	}
	invites, err := store.ListInvites(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(invites) != 0 {
		t.Errorf("%d invites outlived their workspace", len(invites))
	}
}

func TestPostgresRejectsAnUnknownRole(t *testing.T) {
	// The check constraint is the last line of defence if a bad role ever gets
	// past the Go validation.
	store, pool := postgresStore(t)
	insertUser(t, pool, "owner-1")
	ctx := context.Background()

	created, err := serviceOn(store).Create(ctx, "owner-1", "Test", "test")
	if err != nil {
		t.Fatal(err)
	}

	err = store.PutMember(ctx, Member{
		WorkspaceID: created.ID, UserID: "owner-1",
		Role: auth.Role("SUPERUSER"), CreatedAt: time.Now(),
	})
	if err == nil {
		t.Error("the database accepted a role that is not one of the four")
	}
}

func TestPostgresDuplicateSlugIsErrSlugTaken(t *testing.T) {
	// The whole reason this bug existed: the unique constraint lives in
	// Postgres, so only Postgres can prove the driver error is translated. A
	// memory-only test would have passed while production returned a 500.
	store, pool := postgresStore(t)
	insertUser(t, pool, "owner-1")
	insertUser(t, pool, "owner-2")
	ctx := context.Background()
	service := serviceOn(store)

	if _, err := service.Create(ctx, "owner-1", "Switchyard", "switchyard"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Same user, same slug.
	if _, err := service.Create(ctx, "owner-1", "Again", "switchyard"); !errors.Is(err, ErrSlugTaken) {
		t.Errorf("err = %v, want ErrSlugTaken", err)
	}
	// And a stranger, since slugs are global.
	if _, err := service.Create(ctx, "owner-2", "Theirs", "switchyard"); !errors.Is(err, ErrSlugTaken) {
		t.Errorf("err for another user = %v, want ErrSlugTaken", err)
	}

	// A different slug still works, so the translation is not swallowing
	// everything.
	if _, err := service.Create(ctx, "owner-2", "Other", "other"); err != nil {
		t.Errorf("Create with a free slug: %v", err)
	}
}

func TestPostgresAndMemoryAgreeOnDuplicateSlugs(t *testing.T) {
	// The two stores must be interchangeable. This is the assertion that would
	// have caught the original bug at the seam rather than in production.
	store, pool := postgresStore(t)
	insertUser(t, pool, "owner-1")
	ctx := context.Background()

	for name, svc := range map[string]*Service{
		"postgres": serviceOn(store),
		"memory":   NewService(NewMemoryStore()),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Create(ctx, "owner-1", "First", "same-slug"); err != nil {
				t.Fatalf("first Create: %v", err)
			}
			if _, err := svc.Create(ctx, "owner-1", "Second", "same-slug"); !errors.Is(err, ErrSlugTaken) {
				t.Errorf("err = %v, want ErrSlugTaken", err)
			}
		})
	}
}
