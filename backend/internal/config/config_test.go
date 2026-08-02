package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

// testCredentialKey is a valid 32-byte key, base64 as the environment carries
// it. Load requires one, so every case here has to set it.
var testCredentialKey = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", credentialKeySize)))

// testDatabaseURL is never connected to; Load only checks that it is set.
const testDatabaseURL = "postgres://switchyard:switchyard@localhost:5434/switchyard"

// setRequired sets everything Load refuses to start without, so each test only
// has to set the thing it is actually about.
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv(credentialKeyEnv, "1:"+testCredentialKey)
	t.Setenv("DATABASE_URL", testDatabaseURL)
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if got, want := cfg.AuthJWKSURL(), "http://localhost:3007/api/auth/jwks"; got != want {
		t.Errorf("AuthJWKSURL() = %q, want %q", got, want)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	setRequired(t)
	t.Setenv("SWITCHYARD_ADDR", ":9000")
	t.Setenv("SWITCHYARD_AUTH_ISSUER", "https://switchyard.example/")
	t.Setenv("SWITCHYARD_AUTH_AUDIENCE", "other-backend")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":9000" {
		t.Errorf("Addr = %q", cfg.Addr)
	}
	if cfg.AuthAudience != "other-backend" {
		t.Errorf("AuthAudience = %q", cfg.AuthAudience)
	}
	// The trailing slash on the issuer must not produce a doubled one here.
	if got, want := cfg.AuthJWKSURL(), "https://switchyard.example/api/auth/jwks"; got != want {
		t.Errorf("AuthJWKSURL() = %q, want %q", got, want)
	}
}

func TestLoadRejectsBadIssuer(t *testing.T) {
	for _, issuer := range []string{"localhost:3007", "/api", "not a url"} {
		t.Run(issuer, func(t *testing.T) {
			setRequired(t)
			t.Setenv("SWITCHYARD_AUTH_ISSUER", issuer)
			if _, err := Load(); err == nil {
				t.Errorf("Load accepted issuer %q", issuer)
			}
		})
	}
}

func TestLoadReadsCredentialKeys(t *testing.T) {
	older := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("o", credentialKeySize)))
	t.Setenv("DATABASE_URL", testDatabaseURL)
	// Newest first, which is the rule the variable documents.
	t.Setenv(credentialKeyEnv, "2:"+testCredentialKey+", 1:"+older)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CredentialKeyVersion != 2 {
		t.Errorf("CredentialKeyVersion = %d, want 2", cfg.CredentialKeyVersion)
	}
	if len(cfg.CredentialKeys) != 2 {
		t.Fatalf("CredentialKeys has %d entries, want 2", len(cfg.CredentialKeys))
	}
	// The retired key stays loadable, which is the whole point of listing it.
	if got := string(cfg.CredentialKeys[1]); got != strings.Repeat("o", credentialKeySize) {
		t.Errorf("key version 1 decoded to %d bytes of the wrong value", len(got))
	}
}

func TestLoadRejectsBadCredentialKeys(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("too-short"))

	tests := map[string]string{
		"missing":          "",
		"no version":       testCredentialKey,
		"version zero":     "0:" + testCredentialKey,
		"negative version": "-1:" + testCredentialKey,
		"unparsable":       "one:" + testCredentialKey,
		"not base64":       "1:not base64 at all",
		"wrong length":     "1:" + short,
		"duplicate":        "1:" + testCredentialKey + ",1:" + testCredentialKey,
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", testDatabaseURL)
			t.Setenv(credentialKeyEnv, value)
			_, err := Load()
			if err == nil {
				t.Fatalf("Load accepted %s", credentialKeyEnv)
			}
			// The key itself must never be echoed back in the complaint.
			if strings.Contains(err.Error(), testCredentialKey) {
				t.Errorf("error leaks the key: %v", err)
			}
		})
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	// Nothing this server does works without a database, so an unset URL is a
	// startup error rather than a connection failure on the first request.
	t.Setenv(credentialKeyEnv, "1:"+testCredentialKey)
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a missing DATABASE_URL")
	}
}

func TestLoadRejectsBadMaxConns(t *testing.T) {
	for _, value := range []string{"0", "-4", "lots"} {
		t.Run(value, func(t *testing.T) {
			setRequired(t)
			t.Setenv("SWITCHYARD_DB_MAX_CONNS", value)
			if _, err := Load(); err == nil {
				t.Errorf("Load accepted SWITCHYARD_DB_MAX_CONNS=%q", value)
			}
		})
	}
}
