package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDotEnv(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDotEnv(t *testing.T) {
	path := writeDotEnv(t, `
# a comment
DATABASE_URL=postgres://switchyard:switchyard@localhost:5434/switchyard

export SWITCHYARD_ENV=development
QUOTED="value with spaces"
SINGLE='single quoted'
EMPTY=
not a pair
`)

	for _, key := range []string{"DATABASE_URL", "SWITCHYARD_ENV", "QUOTED", "SINGLE", "EMPTY"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}

	want := map[string]string{
		"DATABASE_URL":   "postgres://switchyard:switchyard@localhost:5434/switchyard",
		"SWITCHYARD_ENV": "development",
		"QUOTED":         "value with spaces",
		"SINGLE":         "single quoted",
		"EMPTY":          "",
	}
	for key, expected := range want {
		if got := os.Getenv(key); got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}
}

func TestLoadDotEnvNeverOverridesTheRealEnvironment(t *testing.T) {
	// DATABASE_URL=... go run ./cmd/switchyard has to mean what it says, or a
	// stale file silently sends the server at the wrong database.
	path := writeDotEnv(t, "DATABASE_URL=postgres://from-the-file/db\n")

	t.Setenv("DATABASE_URL", "postgres://from-the-shell/db")
	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}

	if got := os.Getenv("DATABASE_URL"); got != "postgres://from-the-shell/db" {
		t.Errorf("DATABASE_URL = %q, want the shell's value", got)
	}
}

func TestLoadDotEnvIgnoresAMissingFile(t *testing.T) {
	// Production has no .env, and that is not an error.
	if err := loadDotEnv(filepath.Join(t.TempDir(), "nothing-here")); err != nil {
		t.Errorf("loadDotEnv on a missing file: %v", err)
	}
}

func TestParseDotEnvLine(t *testing.T) {
	tests := []struct {
		line  string
		key   string
		value string
		ok    bool
	}{
		{line: "KEY=value", key: "KEY", value: "value", ok: true},
		{line: "  KEY = value  ", key: "KEY", value: "value", ok: true},
		{line: "export KEY=value", key: "KEY", value: "value", ok: true},
		{line: `KEY="two words"`, key: "KEY", value: "two words", ok: true},
		// A base64 key ends in = and must survive being split on the first one.
		{line: "SWITCHYARD_CREDENTIAL_KEY=1:YWJj==", key: "SWITCHYARD_CREDENTIAL_KEY", value: "1:YWJj==", ok: true},
		{line: "# comment", ok: false},
		{line: "", ok: false},
		{line: "   ", ok: false},
		{line: "no equals sign", ok: false},
		{line: "=novalue", ok: false},
	}

	for _, tc := range tests {
		key, value, ok := parseDotEnvLine(tc.line)
		if ok != tc.ok {
			t.Errorf("parseDotEnvLine(%q) ok = %v, want %v", tc.line, ok, tc.ok)
			continue
		}
		if ok && (key != tc.key || value != tc.value) {
			t.Errorf("parseDotEnvLine(%q) = %q, %q; want %q, %q", tc.line, key, value, tc.key, tc.value)
		}
	}
}
