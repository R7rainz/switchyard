package database

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

// migrationLockKey is an arbitrary constant for a Postgres advisory lock. Two
// instances starting together would otherwise both find a migration
// unapplied and both try to run it; the second would fail on an object that
// already exists and take the process down. The lock makes the loser wait and
// then find nothing to do.
const migrationLockKey int64 = 8_675_309

// Migration is one SQL file waiting to be applied.
type Migration struct {
	Version int64
	Name    string
	SQL     string
}

// Migrate applies every migration in files that the database has not seen,
// in version order, and returns the ones it applied.
//
// Each migration runs inside its own transaction together with the row that
// records it, so a failure leaves neither a half-applied schema nor a claim
// that something ran when it did not. Postgres does transactional DDL, which
// is what makes that possible.
func Migrate(ctx context.Context, pool *Pool, files fs.FS) ([]Migration, error) {
	available, err := loadMigrations(files)
	if err != nil {
		return nil, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("database: acquiring a connection: %w", err)
	}
	defer conn.Release()

	// The lock is held on this one connection until it is released, so it must
	// not come from the pool for each statement.
	if _, err := conn.Exec(ctx, "select pg_advisory_lock($1)", migrationLockKey); err != nil {
		return nil, fmt.Errorf("database: taking the migration lock: %w", err)
	}
	defer func() {
		// Best effort: if this fails the connection is being torn down anyway,
		// and Postgres drops advisory locks when the session ends.
		_, _ = conn.Exec(context.WithoutCancel(ctx), "select pg_advisory_unlock($1)", migrationLockKey)
	}()

	if _, err := conn.Exec(ctx, `
		create table if not exists "schema_migrations" (
			"version"   bigint primary key,
			"name"      text not null,
			"appliedAt" timestamptz default CURRENT_TIMESTAMP not null
		)`); err != nil {
		return nil, fmt.Errorf("database: creating the migration table: %w", err)
	}

	rows, err := conn.Query(ctx, `select "version" from "schema_migrations"`)
	if err != nil {
		return nil, fmt.Errorf("database: reading applied migrations: %w", err)
	}
	applied := make(map[int64]bool)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			rows.Close()
			return nil, fmt.Errorf("database: reading applied migrations: %w", err)
		}
		applied[version] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: reading applied migrations: %w", err)
	}

	var ran []Migration
	for _, migration := range available {
		if applied[migration.Version] {
			continue
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return ran, fmt.Errorf("database: starting migration %d: %w", migration.Version, err)
		}

		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return ran, fmt.Errorf("database: migration %d (%s): %w", migration.Version, migration.Name, err)
		}
		if _, err := tx.Exec(ctx,
			`insert into "schema_migrations" ("version", "name") values ($1, $2)`,
			migration.Version, migration.Name,
		); err != nil {
			_ = tx.Rollback(ctx)
			return ran, fmt.Errorf("database: recording migration %d: %w", migration.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return ran, fmt.Errorf("database: committing migration %d: %w", migration.Version, err)
		}

		ran = append(ran, migration)
	}
	return ran, nil
}

// loadMigrations reads and orders the .sql files, rejecting anything it cannot
// give a version number.
func loadMigrations(files fs.FS) ([]Migration, error) {
	entries, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("database: listing migrations: %w", err)
	}

	found := make([]Migration, 0, len(entries))
	seen := make(map[int64]string, len(entries))

	for _, entry := range entries {
		version, err := versionOf(entry)
		if err != nil {
			return nil, err
		}
		// Two files claiming one version would apply in an order that depends
		// on the filesystem, and only one would ever be recorded.
		if other, clash := seen[version]; clash {
			return nil, fmt.Errorf("database: migrations %q and %q share version %d", other, entry, version)
		}
		seen[version] = entry

		body, err := fs.ReadFile(files, entry)
		if err != nil {
			return nil, fmt.Errorf("database: reading %s: %w", entry, err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("database: migration %s is empty", entry)
		}

		found = append(found, Migration{Version: version, Name: entry, SQL: string(body)})
	}

	// Glob's order is lexical, which puts 10 before 9. Sort on the number.
	sort.Slice(found, func(i, j int) bool { return found[i].Version < found[j].Version })
	return found, nil
}

// versionOf reads the leading number off a name like 0002_credentials.sql.
func versionOf(name string) (int64, error) {
	base := path.Base(name)
	digits := base
	if cut := strings.IndexByte(base, '_'); cut >= 0 {
		digits = base[:cut]
	} else {
		digits = strings.TrimSuffix(base, ".sql")
	}

	version, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("database: migration %q must start with a positive number, as in 0001_name.sql", base)
	}
	return version, nil
}

// ErrDirty reports a database whose recorded migrations do not match the files
// the binary carries.
var ErrDirty = errors.New("database: applied migrations do not match this build")

// Verify reports migrations the database has applied that this binary does not
// know about, which means it is older than the schema it is pointed at.
//
// Running the previous version against a migrated database is the usual way a
// deploy goes wrong quietly, so it is worth naming rather than discovering
// through a column that is not there.
func Verify(ctx context.Context, pool *Pool, files fs.FS) error {
	available, err := loadMigrations(files)
	if err != nil {
		return err
	}
	known := make(map[int64]bool, len(available))
	for _, migration := range available {
		known[migration.Version] = true
	}

	rows, err := pool.Query(ctx, `select "version" from "schema_migrations" order by "version"`)
	if err != nil {
		return fmt.Errorf("database: reading applied migrations: %w", err)
	}
	defer rows.Close()

	var unknown []int64
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("database: reading applied migrations: %w", err)
		}
		if !known[version] {
			unknown = append(unknown, version)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("database: reading applied migrations: %w", err)
	}

	if len(unknown) > 0 {
		return fmt.Errorf("%w: the database has %v, which this binary does not carry", ErrDirty, unknown)
	}
	return nil
}
