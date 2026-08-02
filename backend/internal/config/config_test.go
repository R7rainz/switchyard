package config

import "testing"

func TestLoadDefaults(t *testing.T) {
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
			t.Setenv("SWITCHYARD_AUTH_ISSUER", issuer)
			if _, err := Load(); err == nil {
				t.Errorf("Load accepted issuer %q", issuer)
			}
		})
	}
}
