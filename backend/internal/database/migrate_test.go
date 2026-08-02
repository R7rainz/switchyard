package database

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadMigrationsOrdersByNumberNotName(t *testing.T) {
	// Lexical order puts 10 before 9, which would apply migrations in the
	// wrong order the first time the tenth one is written.
	files := fstest.MapFS{
		"0010_tenth.sql": {Data: []byte("select 10")},
		"0009_ninth.sql": {Data: []byte("select 9")},
		"0001_first.sql": {Data: []byte("select 1")},
	}

	found, err := loadMigrations(files)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}

	want := []int64{1, 9, 10}
	if len(found) != len(want) {
		t.Fatalf("found %d migrations, want %d", len(found), len(want))
	}
	for i, version := range want {
		if found[i].Version != version {
			t.Errorf("position %d is version %d, want %d", i, found[i].Version, version)
		}
	}
}

func TestLoadMigrationsRejects(t *testing.T) {
	tests := map[string]fstest.MapFS{
		"no leading number": {
			"credentials.sql": {Data: []byte("select 1")},
		},
		"zero version": {
			"0000_zero.sql": {Data: []byte("select 1")},
		},
		"empty file": {
			"0001_empty.sql": {Data: []byte("   \n")},
		},
		// Two files at one version apply in filesystem order and only one gets
		// recorded, so the other silently never runs again.
		"duplicate version": {
			"0001_first.sql":  {Data: []byte("select 1")},
			"0001_second.sql": {Data: []byte("select 2")},
		},
	}

	for name, files := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := loadMigrations(files); err == nil {
				t.Error("loadMigrations accepted it")
			}
		})
	}
}

func TestVersionOf(t *testing.T) {
	tests := map[string]int64{
		"0001_better_auth.sql":               1,
		"0002_credentials.sql":               2,
		"0003_workspaces.sql":                3,
		"migrations/0042_something_long.sql": 42,
		"7.sql":                              7,
	}

	for name, want := range tests {
		got, err := versionOf(name)
		if err != nil {
			t.Errorf("versionOf(%q): %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("versionOf(%q) = %d, want %d", name, got, want)
		}
	}
}

// The real migrations must load, or the binary cannot start. This catches a
// badly named or empty file at test time rather than at boot.
func TestRealMigrationsAreWellFormed(t *testing.T) {
	found, err := loadMigrations(realMigrations(t))
	if err != nil {
		t.Fatalf("the shipped migrations do not load: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no migrations found")
	}

	for i, migration := range found {
		if i > 0 && migration.Version == found[i-1].Version {
			t.Errorf("two migrations share version %d", migration.Version)
		}
		if !strings.HasSuffix(migration.Name, ".sql") {
			t.Errorf("migration %q is not a .sql file", migration.Name)
		}
	}
}
