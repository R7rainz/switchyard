package credential

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/R7rainz/switchyard/backend/internal/database"
	"github.com/R7rainz/switchyard/backend/migrations"
)

// postgresStore connects to SWITCHYARD_TEST_DATABASE_URL and migrates it,
// skipping when unset so `go test ./...` stays offline.
//
// It drops and recreates the public schema. Never point it at a database
// anyone cares about.
func postgresStore(t *testing.T) (*PostgresStore, *pgxpool.Pool) {
	t.Helper()

	url := os.Getenv("SWITCHYARD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set SWITCHYARD_TEST_DATABASE_URL to run the Postgres store tests")
	}
	// These tests drop the public schema. Pointed at the development database
	// that is every workflow, execution, and user gone, and the two URLs are
	// similar enough to paste one where the other belongs.
	if err := database.CheckTestURL(url, os.Getenv("DATABASE_URL")); err != nil {
		t.Fatal(err)
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

// insertWorkspace creates the row credentials reference. Since 0004 the
// foreign key points at "workspace", not "user".
func insertWorkspace(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`insert into "workspace" ("id", "name", "slug") values ($1, $1, $1)`, id)
	if err != nil {
		t.Fatalf("inserting workspace %s: %v", id, err)
	}
}

// serviceOn wires the real service over Postgres, so the crypto and the SQL
// are exercised together.
func serviceOn(t *testing.T, store *PostgresStore) *Service {
	t.Helper()
	ring, err := NewKeyring(1, map[int][]byte{1: testKey(t)})
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	return NewService(store, ring)
}

func TestPostgresPutGetRoundTrip(t *testing.T) {
	store, pool := postgresStore(t)
	insertWorkspace(t, pool, "ws-1")
	ctx := context.Background()
	service := serviceOn(t, store)

	if err := service.Put(ctx, "ws-1", "github", "ci", Secret("ghp_real")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := service.Get(ctx, "ws-1", "github", "ci")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "ghp_real" {
		t.Errorf("Get = %q, want the stored secret", got)
	}
}

func TestPostgresStoresNoPlaintext(t *testing.T) {
	// The row is what an attacker with database access sees, so the secret
	// must not be in it.
	store, pool := postgresStore(t)
	insertWorkspace(t, pool, "ws-1")
	ctx := context.Background()

	if err := serviceOn(t, store).Put(ctx, "ws-1", "github", "ci", Secret("ghp_plaintext")); err != nil {
		t.Fatal(err)
	}

	var ciphertext []byte
	if err := pool.QueryRow(ctx,
		`select "ciphertext" from "credentials" where "workspaceId" = $1`, "ws-1",
	).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("ghp_plaintext")) {
		t.Error("the stored ciphertext contains the plaintext")
	}
}

func TestPostgresSaveReplacesKeepingIdentity(t *testing.T) {
	// Without ON CONFLICT the unique triple would reject every update. The id
	// has to survive too: it is what an operator sees in a rotation failure.
	store, pool := postgresStore(t)
	insertWorkspace(t, pool, "ws-1")
	ctx := context.Background()
	service := serviceOn(t, store)

	if err := service.Put(ctx, "ws-1", "github", "ci", Secret("first")); err != nil {
		t.Fatal(err)
	}
	before, err := store.Get(ctx, "ws-1", "github", "ci")
	if err != nil {
		t.Fatal(err)
	}

	if err := service.Put(ctx, "ws-1", "github", "ci", Secret("second")); err != nil {
		t.Fatalf("replacing: %v", err)
	}

	all, err := store.List(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("workspace holds %d credentials, want 1 — the upsert duplicated", len(all))
	}

	after, err := store.Get(ctx, "ws-1", "github", "ci")
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID {
		t.Errorf("id changed from %s to %s on replace", before.ID, after.ID)
	}
	if !after.CreatedAt.Equal(before.CreatedAt) {
		t.Errorf("createdAt moved on replace")
	}

	secret, err := service.Get(ctx, "ws-1", "github", "ci")
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != "second" {
		t.Errorf("Get = %q, want the replacement", secret)
	}
}

func TestPostgresOneWorkspaceCannotReadAnother(t *testing.T) {
	store, pool := postgresStore(t)
	insertWorkspace(t, pool, "ws-alpha")
	insertWorkspace(t, pool, "ws-beta")
	ctx := context.Background()
	service := serviceOn(t, store)

	if err := service.Put(ctx, "ws-alpha", "github", "ci", Secret("alpha-secret")); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Get(ctx, "ws-beta", "github", "ci"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get across workspaces = %v, want ErrNotFound", err)
	}
	if err := service.Delete(ctx, "ws-beta", "github", "ci"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete across workspaces = %v, want ErrNotFound", err)
	}
	listed, err := store.List(ctx, "ws-beta")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Errorf("List returned %d of another workspace's credentials", len(listed))
	}
}

func TestPostgresRotateMakesProgress(t *testing.T) {
	// Stale must stop returning a row once it is re-sealed, or Rotate loops on
	// the same batch forever.
	store, pool := postgresStore(t)
	insertWorkspace(t, pool, "ws-1")
	ctx := context.Background()

	oldKey, newKey := testKey(t), testKey(t)
	oldRing, err := NewKeyring(1, map[int][]byte{1: oldKey})
	if err != nil {
		t.Fatal(err)
	}
	writer := NewService(store, oldRing)

	for _, name := range []string{"one", "two", "three"} {
		if err := writer.Put(ctx, "ws-1", "github", name, Secret("secret-"+name)); err != nil {
			t.Fatal(err)
		}
	}

	newRing, err := NewKeyring(2, map[int][]byte{1: oldKey, 2: newKey})
	if err != nil {
		t.Fatal(err)
	}
	rotator := NewService(store, newRing)

	// Batches of two, so this takes more than one call.
	total := 0
	for range 5 {
		n, err := rotator.Rotate(ctx, 2)
		if err != nil {
			t.Fatalf("Rotate: %v", err)
		}
		total += n
		if n == 0 {
			break
		}
	}
	if total != 3 {
		t.Fatalf("rotated %d records, want 3", total)
	}

	// Everything is on the new key and still readable.
	for _, name := range []string{"one", "two", "three"} {
		record, err := store.Get(ctx, "ws-1", "github", name)
		if err != nil {
			t.Fatal(err)
		}
		if record.KeyVersion != 2 {
			t.Errorf("%s is still on key version %d", name, record.KeyVersion)
		}
		secret, err := rotator.Get(ctx, "ws-1", "github", name)
		if err != nil {
			t.Fatalf("reading %s after rotation: %v", name, err)
		}
		if string(secret) != "secret-"+name {
			t.Errorf("%s = %q after rotation", name, secret)
		}
	}
}

func TestPostgresStaleRejectsANonPositiveLimit(t *testing.T) {
	// A limit of zero would silently return nothing, which reads as "rotation
	// complete" and retires a key that is still in use.
	store, _ := postgresStore(t)
	if _, err := store.Stale(context.Background(), 1, 0); err == nil {
		t.Error("Stale accepted a limit of 0")
	}
}

func TestPostgresCredentialsDieWithTheirWorkspace(t *testing.T) {
	store, pool := postgresStore(t)
	insertWorkspace(t, pool, "ws-1")
	ctx := context.Background()

	if err := serviceOn(t, store).Put(ctx, "ws-1", "github", "ci", Secret("s")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `delete from "workspace" where "id" = $1`, "ws-1"); err != nil {
		t.Fatalf("deleting workspace: %v", err)
	}

	listed, err := store.List(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Errorf("%d credentials outlived their workspace", len(listed))
	}
}

func TestPostgresRejectsAnUnknownWorkspace(t *testing.T) {
	// The foreign key is what stops a credential being stranded with no owner.
	store, _ := postgresStore(t)
	if err := serviceOn(t, store).Put(context.Background(), "no-such-workspace", "github", "ci", Secret("s")); err == nil {
		t.Error("saved a credential for a workspace that does not exist")
	}
}
