package workflow

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/R7rainz/switchyard/backend/internal/database"
	"github.com/R7rainz/switchyard/backend/migrations"
)

// The two stores must be interchangeable, so the expectations are written once
// and run against both. The slug bug that shipped in the workspace package came
// from a fake that accepted what Postgres refused; this is the shape of test
// that catches that class before it leaves the machine.
func TestStoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		runStoreContract(t, func(t *testing.T) Store { return NewMemoryStore() })
	})
	t.Run("postgres", func(t *testing.T) {
		runStoreContract(t, func(t *testing.T) Store { return postgresStore(t) })
	})
}

func sample(id, workspaceID, name string) Workflow {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	return Workflow{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        name,
		Description: "made by a test",
		Graph:       validGraph(),
		CreatedBy:   user,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func runStoreContract(t *testing.T, newStore func(*testing.T) Store) {
	ctx := context.Background()

	t.Run("round trips a graph", func(t *testing.T) {
		store := newStore(t)
		want := sample("wf-1", wsA, "first")
		if err := store.Create(ctx, want); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := store.Get(ctx, wsA, "wf-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Name != want.Name || got.Description != want.Description || got.CreatedBy != want.CreatedBy {
			t.Fatalf("got %+v, want %+v", got, want)
		}
		if len(got.Graph.Nodes) != len(want.Graph.Nodes) || len(got.Graph.Edges) != len(want.Graph.Edges) {
			t.Fatalf("graph did not survive the round trip: %+v", got.Graph)
		}
		if got.Graph.Nodes[0].Type != want.Graph.Nodes[0].Type {
			t.Fatalf("node type changed: %q", got.Graph.Nodes[0].Type)
		}
	})

	// The whole reason every Store method takes a workspace id.
	t.Run("another workspace cannot reach it", func(t *testing.T) {
		store := newStore(t)
		if err := store.Create(ctx, sample("wf-1", wsA, "first")); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if _, err := store.Get(ctx, wsB, "wf-1"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get from the wrong workspace: got %v, want ErrNotFound", err)
		}
		if err := store.Delete(ctx, wsB, "wf-1"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Delete from the wrong workspace: got %v, want ErrNotFound", err)
		}
		wrong := sample("wf-1", wsB, "renamed by an outsider")
		if err := store.Update(ctx, wrong); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Update from the wrong workspace: got %v, want ErrNotFound", err)
		}

		// And the original is untouched by all three.
		got, err := store.Get(ctx, wsA, "wf-1")
		if err != nil || got.Name != "first" {
			t.Fatalf("original changed: %+v, %v", got, err)
		}
	})

	t.Run("lists only its own workspace, by name", func(t *testing.T) {
		store := newStore(t)
		for _, w := range []Workflow{
			sample("wf-2", wsA, "beta"),
			sample("wf-1", wsA, "alpha"),
			sample("wf-3", wsB, "elsewhere"),
		} {
			if err := store.Create(ctx, w); err != nil {
				t.Fatalf("Create: %v", err)
			}
		}

		got, err := store.List(ctx, wsA)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d workflows, want 2", len(got))
		}
		if got[0].Name != "alpha" || got[1].Name != "beta" {
			t.Fatalf("got %q then %q, want them ordered by name", got[0].Name, got[1].Name)
		}
	})

	t.Run("missing rows are reported, not swallowed", func(t *testing.T) {
		store := newStore(t)
		if _, err := store.Get(ctx, wsA, "ghost"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get: got %v, want ErrNotFound", err)
		}
		if err := store.Delete(ctx, wsA, "ghost"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Delete: got %v, want ErrNotFound", err)
		}
		if err := store.Update(ctx, sample("ghost", wsA, "nope")); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Update: got %v, want ErrNotFound", err)
		}
		if got, err := store.List(ctx, wsA); err != nil || len(got) != 0 {
			t.Fatalf("List on an empty workspace: got %v, %v", got, err)
		}
	})

	t.Run("update replaces the graph", func(t *testing.T) {
		store := newStore(t)
		if err := store.Create(ctx, sample("wf-1", wsA, "first")); err != nil {
			t.Fatalf("Create: %v", err)
		}

		changed := sample("wf-1", wsA, "renamed")
		changed.Graph = Graph{Nodes: []Node{node("t", "trigger.webhook")}}
		if err := store.Update(ctx, changed); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := store.Get(ctx, wsA, "wf-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Name != "renamed" || len(got.Graph.Nodes) != 1 || len(got.Graph.Edges) != 0 {
			t.Fatalf("update did not land: %+v", got)
		}
	})
}

// postgresStore connects to SWITCHYARD_TEST_DATABASE_URL, migrates it, clears
// it, and seeds the workspaces and user the foreign keys need. It skips when
// the variable is unset, so the normal `go test ./...` stays offline.
//
// It drops and recreates the public schema, so never point it at a database
// anyone cares about.
func postgresStore(t *testing.T) *PostgresStore {
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

	// go test runs packages in parallel, and several of them reset this same
	// schema. Without the lock they tear the database out from under each
	// other. Postgres drops it when the pool closes.
	if _, err := pool.Exec(ctx, "select pg_advisory_lock($1)", database.TestSchemaLock); err != nil {
		t.Fatalf("taking the test schema lock: %v", err)
	}
	if _, err := pool.Exec(ctx, "drop schema public cascade; create schema public"); err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
	if _, err := database.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	seed(t, pool)
	return NewPostgresStore(pool)
}

// seed inserts the rows workflow's foreign keys point at. The memory store has
// no such requirement, which is fine: the contract above asserts behaviour both
// stores owe a caller, not the setup each needs.
func seed(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`insert into "user" ("id", "name", "email", "emailVerified") values ($1, $1, $1 || '@test.local', false)`,
		user)
	if err != nil {
		t.Fatalf("inserting user: %v", err)
	}

	for _, id := range []string{wsA, wsB} {
		if _, err := pool.Exec(ctx,
			`insert into "workspace" ("id", "name", "slug") values ($1, $1, $1)`, id); err != nil {
			t.Fatalf("inserting workspace %s: %v", id, err)
		}
	}
}
