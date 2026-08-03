package execution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/R7rainz/switchyard/backend/internal/database"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
	"github.com/R7rainz/switchyard/backend/migrations"
)

// One set of expectations, both stores. A fake more permissive than the real
// one is how a bug ships green.
func TestStoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		runStoreContract(t, func(t *testing.T) Store { return NewMemoryStore() })
	})
	t.Run("postgres", func(t *testing.T) {
		runStoreContract(t, func(t *testing.T) Store { return postgresStore(t) })
	})
}

var baseTime = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

func sampleRun(id, workspaceID string, at time.Time) Execution {
	return Execution{
		ID:          id,
		WorkspaceID: workspaceID,
		WorkflowID:  "wf-1",
		Graph: workflow.Graph{
			Nodes: []workflow.Node{node("t", "trigger.manual", `{"label":"Manually"}`)},
		},
		Status:    StatusPending,
		Trigger:   TriggerManual,
		Input:     json.RawMessage(`{"repo":"main"}`),
		StartedBy: user,
		CreatedAt: at,
	}
}

func runStoreContract(t *testing.T, newStore func(*testing.T) Store) {
	ctx := context.Background()

	t.Run("round trips a run and its snapshot", func(t *testing.T) {
		store := newStore(t)
		if err := store.Create(ctx, sampleRun("r1", wsA, baseTime)); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := store.Get(ctx, wsA, "r1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Status != StatusPending || got.Trigger != TriggerManual || got.StartedBy != user {
			t.Fatalf("got %+v", got)
		}
		if len(got.Graph.Nodes) != 1 || got.Graph.Nodes[0].ID != "t" {
			t.Fatalf("snapshot did not survive: %+v", got.Graph)
		}
		// Compared as JSON, not as bytes: jsonb normalises whitespace on the
		// way in, and it is the value that matters, not its formatting.
		if !jsonEq(t, got.Input, `{"repo":"main"}`) {
			t.Fatalf("input = %s", got.Input)
		}
	})

	t.Run("another workspace cannot reach it", func(t *testing.T) {
		store := newStore(t)
		if err := store.Create(ctx, sampleRun("r1", wsA, baseTime)); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := store.Get(ctx, wsB, "r1"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
		if runs, err := store.List(ctx, wsB, "", 0); err != nil || len(runs) != 0 {
			t.Fatalf("List: %v, %v", runs, err)
		}
	})

	t.Run("lists newest first and filters by workflow", func(t *testing.T) {
		store := newStore(t)
		for i, at := range []time.Time{baseTime, baseTime.Add(time.Minute), baseTime.Add(2 * time.Minute)} {
			run := sampleRun(string(rune('a'+i)), wsA, at)
			if i == 2 {
				run.WorkflowID = "wf-2"
			}
			if err := store.Create(ctx, run); err != nil {
				t.Fatalf("Create: %v", err)
			}
		}

		all, err := store.List(ctx, wsA, "", 0)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(all) != 3 || all[0].ID != "c" || all[2].ID != "a" {
			t.Fatalf("order wrong: %v", ids(all))
		}

		filtered, err := store.List(ctx, wsA, "wf-2", 0)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(filtered) != 1 || filtered[0].ID != "c" {
			t.Fatalf("filter wrong: %v", ids(filtered))
		}

		limited, err := store.List(ctx, wsA, "", 2)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(limited) != 2 {
			t.Fatalf("limit ignored: %v", ids(limited))
		}
	})

	t.Run("start then finish", func(t *testing.T) {
		store := newStore(t)
		if err := store.Create(ctx, sampleRun("r1", wsA, baseTime)); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := store.Start(ctx, "r1", baseTime.Add(time.Second)); err != nil {
			t.Fatalf("Start: %v", err)
		}
		running, _ := store.Get(ctx, wsA, "r1")
		if running.Status != StatusRunning || running.StartedAt.IsZero() {
			t.Fatalf("got %+v", running)
		}

		if err := store.Finish(ctx, "r1", StatusSucceeded, "", baseTime.Add(2*time.Second)); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		done, _ := store.Get(ctx, wsA, "r1")
		if done.Status != StatusSucceeded || done.FinishedAt.IsZero() {
			t.Fatalf("got %+v", done)
		}
	})

	t.Run("node runs upsert on the node id", func(t *testing.T) {
		store := newStore(t)
		if err := store.Create(ctx, sampleRun("r1", wsA, baseTime)); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// The engine writes once when a node starts and again when it ends.
		// Two rows for one node would make the viewer show it twice.
		start := NodeRun{ExecutionID: "r1", NodeID: "t", Status: StatusRunning, StartedAt: baseTime}
		if err := store.SaveNodeRun(ctx, start); err != nil {
			t.Fatalf("SaveNodeRun: %v", err)
		}
		end := start
		end.Status = StatusSucceeded
		end.Output = json.RawMessage(`{"ok":true}`)
		end.FinishedAt = baseTime.Add(time.Second)
		if err := store.SaveNodeRun(ctx, end); err != nil {
			t.Fatalf("SaveNodeRun: %v", err)
		}

		rows, err := store.NodeRuns(ctx, "r1")
		if err != nil {
			t.Fatalf("NodeRuns: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("got %d rows, want 1", len(rows))
		}
		if rows[0].Status != StatusSucceeded || !jsonEq(t, rows[0].Output, `{"ok":true}`) {
			t.Fatalf("got %+v, output %s", rows[0], rows[0].Output)
		}
	})

	t.Run("reclaim fails only unfinished runs", func(t *testing.T) {
		store := newStore(t)
		for id, status := range map[string]Status{
			"r1": StatusPending, "r2": StatusRunning, "r3": StatusSucceeded, "r4": StatusFailed,
		} {
			run := sampleRun(id, wsA, baseTime)
			run.Status = status
			if err := store.Create(ctx, run); err != nil {
				t.Fatalf("Create: %v", err)
			}
		}

		count, err := store.Reclaim(ctx, "restarted")
		if err != nil {
			t.Fatalf("Reclaim: %v", err)
		}
		if count != 2 {
			t.Fatalf("reclaimed %d, want 2", count)
		}
		done, _ := store.Get(ctx, wsA, "r3")
		if done.Status != StatusSucceeded {
			t.Fatalf("a finished run was reclaimed: %s", done.Status)
		}
	})

	// The guard that stops a node finishing just after a cancel from
	// overwriting CANCELLED with SUCCEEDED.
	t.Run("finish does not overwrite a terminal status", func(t *testing.T) {
		store := newStore(t)
		if err := store.Create(ctx, sampleRun("r1", wsA, baseTime)); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := store.Finish(ctx, "r1", StatusCancelled, "cancelled", baseTime); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		if err := store.Finish(ctx, "r1", StatusSucceeded, "", baseTime.Add(time.Second)); err != nil {
			t.Fatalf("Finish: %v", err)
		}

		got, _ := store.Get(ctx, wsA, "r1")
		if got.Status != StatusCancelled {
			t.Fatalf("status = %s, want the cancel to stand", got.Status)
		}
	})
}

// jsonEq compares two JSON documents by value. Postgres stores jsonb parsed
// rather than as text, so it hands back its own formatting.
func jsonEq(t *testing.T, got json.RawMessage, want string) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("test wants invalid JSON: %s", want)
	}
	return reflect.DeepEqual(a, b)
}

func ids(runs []Execution) []string {
	found := make([]string, len(runs))
	for i, run := range runs {
		found[i] = run.ID
	}
	return found
}

// postgresStore connects to SWITCHYARD_TEST_DATABASE_URL, migrates it, clears
// it, and seeds the rows the foreign keys need. It skips when the variable is
// unset, so the normal `go test ./...` stays offline.
//
// It drops and recreates the public schema, so never point it at a database
// anyone cares about.
func postgresStore(t *testing.T) *PostgresStore {
	t.Helper()

	url := os.Getenv("SWITCHYARD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set SWITCHYARD_TEST_DATABASE_URL to run the Postgres store tests")
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, database.Options{URL: url, MaxConns: 1, ConnectTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(pool.Close)

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

func seed(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`insert into "user" ("id", "name", "email", "emailVerified") values ($1, $1, $1 || '@test.local', false)`,
		user); err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	for _, id := range []string{wsA, wsB} {
		if _, err := pool.Exec(ctx,
			`insert into "workspace" ("id", "name", "slug") values ($1, $1, $1)`, id); err != nil {
			t.Fatalf("inserting workspace %s: %v", id, err)
		}
	}
	for _, id := range []string{"wf-1", "wf-2"} {
		if _, err := pool.Exec(ctx,
			`insert into "workflow" ("id", "workspaceId", "name", "graph") values ($1, $2, $1, '{}')`,
			id, wsA); err != nil {
			t.Fatalf("inserting workflow %s: %v", id, err)
		}
	}
}
