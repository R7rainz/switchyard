package database

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/R7rainz/switchyard/backend/migrations"
)

// realMigrations returns the migrations the binary actually ships.
func realMigrations(t *testing.T) fs.FS {
	t.Helper()
	return migrations.FS
}

// testPool connects to the database named by SWITCHYARD_TEST_DATABASE_URL, or
// skips. These tests create and drop tables, so they must never be pointed at
// a database anyone cares about.
func testPool(t *testing.T) *Pool {
	t.Helper()

	url := os.Getenv("SWITCHYARD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set SWITCHYARD_TEST_DATABASE_URL to run migration tests against a real database")
	}

	// These tests drop the public schema. Pointed at the development database
	// that is every workflow, execution, and user gone, and the two URLs are
	// similar enough to paste one where the other belongs.
	if err := CheckTestURL(url, os.Getenv("DATABASE_URL")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// One connection, so the advisory lock is held on it for the whole test.
	pool, err := Connect(ctx, Options{URL: url, MaxConns: 1, ConnectTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(pool.Close)

	// The workspace and credential store tests reset this same database, and
	// go test runs those packages in parallel with this one.
	if _, err := pool.Exec(ctx, "select pg_advisory_lock($1)", TestSchemaLock); err != nil {
		t.Fatalf("taking the test schema lock: %v", err)
	}
	return pool
}

// freshSchema drops and recreates public, so each test starts from nothing.
func freshSchema(t *testing.T, pool *Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "drop schema public cascade; create schema public"); err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
}

func TestMigrateAppliesEverythingThenNothing(t *testing.T) {
	pool := testPool(t)
	freshSchema(t, pool)
	ctx := context.Background()

	applied, err := Migrate(ctx, pool, migrations.FS)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("first run applied nothing")
	}

	// Every table the shipped migrations create should now exist. credentials
	// is the one that was missing from the dev database, because the old
	// docker-entrypoint seeding only ran on an empty volume.
	for _, table := range []string{"user", "session", "jwks", "credentials", "workspace", "workspace_member", "workspace_invite"} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`select exists (select 1 from pg_tables where schemaname = 'public' and tablename = $1)`,
			table,
		).Scan(&exists); err != nil {
			t.Fatalf("checking for %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q was not created", table)
		}
	}

	// Running again must be a no-op, since that is what happens on every
	// restart.
	again, err := Migrate(ctx, pool, migrations.FS)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second run applied %d migrations, want 0", len(again))
	}
}

func TestMigrateAppliesOnlyWhatIsMissing(t *testing.T) {
	// The case the old setup got wrong: a database that already has some
	// migrations must receive exactly the new ones.
	pool := testPool(t)
	freshSchema(t, pool)
	ctx := context.Background()

	first := fstest.MapFS{
		"0001_first.sql": {Data: []byte(`create table "one" ("id" text primary key)`)},
	}
	if _, err := Migrate(ctx, pool, first); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	both := fstest.MapFS{
		"0001_first.sql":  {Data: []byte(`create table "one" ("id" text primary key)`)},
		"0002_second.sql": {Data: []byte(`create table "two" ("id" text primary key)`)},
	}
	applied, err := Migrate(ctx, pool, both)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(applied) != 1 || applied[0].Version != 2 {
		t.Fatalf("applied %v, want only version 2", applied)
	}
}

func TestFailedMigrationLeavesNoTrace(t *testing.T) {
	// The migration and the row recording it share a transaction, so a broken
	// migration must roll back both — otherwise a half-applied schema is
	// marked done and the next start skips it.
	pool := testPool(t)
	freshSchema(t, pool)
	ctx := context.Background()

	broken := fstest.MapFS{
		"0001_ok.sql":     {Data: []byte(`create table "kept" ("id" text primary key)`)},
		"0002_broken.sql": {Data: []byte(`create table "half" ("id" text primary key); this is not sql`)},
	}

	applied, err := Migrate(ctx, pool, broken)
	if err == nil {
		t.Fatal("Migrate accepted a broken migration")
	}
	if len(applied) != 1 || applied[0].Version != 1 {
		t.Errorf("applied = %v, want the first one only", applied)
	}

	var half bool
	if err := pool.QueryRow(ctx,
		`select exists (select 1 from pg_tables where schemaname='public' and tablename='half')`,
	).Scan(&half); err != nil {
		t.Fatal(err)
	}
	if half {
		t.Error("the failed migration left its table behind")
	}

	var recorded int
	if err := pool.QueryRow(ctx, `select count(*) from "schema_migrations" where "version" = 2`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 0 {
		t.Error("the failed migration was recorded as applied")
	}
}

func TestVerifyDetectsASchemaFromTheFuture(t *testing.T) {
	// Rolling the binary back without rolling the schema back is the quiet
	// failure worth naming.
	pool := testPool(t)
	freshSchema(t, pool)
	ctx := context.Background()

	newer := fstest.MapFS{
		"0001_first.sql":  {Data: []byte(`create table "one" ("id" text primary key)`)},
		"0002_second.sql": {Data: []byte(`create table "two" ("id" text primary key)`)},
	}
	if _, err := Migrate(ctx, pool, newer); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	older := fstest.MapFS{
		"0001_first.sql": {Data: []byte(`create table "one" ("id" text primary key)`)},
	}
	if err := Verify(ctx, pool, older); err == nil {
		t.Error("Verify accepted a database ahead of the binary")
	}

	if err := Verify(ctx, pool, newer); err != nil {
		t.Errorf("Verify rejected a matching database: %v", err)
	}
}

func TestConnectFailsFastOnAnUnreachableServer(t *testing.T) {
	// pgxpool connects lazily, so without the ping in Connect this would look
	// like a healthy start and fail on the first query instead.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := Connect(ctx, Options{
		URL:            "postgres://nobody:nobody@127.0.0.1:1/nothing",
		ConnectTimeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatal("Connect reported success against a server that is not there")
	}
}

func TestConnectRejectsAnEmptyURL(t *testing.T) {
	if _, err := Connect(context.Background(), Options{}); err == nil {
		t.Fatal("Connect accepted an empty URL")
	}
}
