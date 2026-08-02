package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// Config is the runtime configuration for the API server.
type Config struct {
	// Addr is the listen address, in net.Listen form.
	Addr string

	// AuthIssuer is the frontend's public base URL. Better Auth stamps it into
	// every token as "iss", so this must match what the frontend serves on.
	AuthIssuer string

	// AuthAudience is the "aud" this backend requires in a token.
	AuthAudience string

	// Development selects human-readable logs over JSON. It is the local
	// default; anything deployed should set SWITCHYARD_ENV=production so logs
	// stay machine-parseable.
	Development bool

	// LogLevel is the minimum level that reaches the log.
	LogLevel zerolog.Level
}

// AuthJWKSURL is where the issuer publishes its public keys.
func (c Config) AuthJWKSURL() string {
	return strings.TrimSuffix(c.AuthIssuer, "/") + "/api/auth/jwks"
}

// Load reads configuration from the environment.
//
// Defaults suit local development against the docker-compose stack; anything
// that would leave the server misconfigured is an error rather than a silent
// fallback.
func Load() (Config, error) {
	cfg := Config{
		Addr:         envOr("SWITCHYARD_ADDR", ":8080"),
		AuthIssuer:   envOr("SWITCHYARD_AUTH_ISSUER", "http://localhost:3007"),
		AuthAudience: envOr("SWITCHYARD_AUTH_AUDIENCE", "switchyard-backend"),
		Development:  envOr("SWITCHYARD_ENV", "development") != "production",
	}

	level, err := zerolog.ParseLevel(envOr("SWITCHYARD_LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, fmt.Errorf("config: SWITCHYARD_LOG_LEVEL: %w", err)
	}
	cfg.LogLevel = level

	// A relative or malformed issuer would make every token comparison fail in
	// a way that looks like a bad token, so reject it at startup instead.
	parsed, err := url.Parse(cfg.AuthIssuer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Config{}, fmt.Errorf("config: SWITCHYARD_AUTH_ISSUER %q is not an absolute URL", cfg.AuthIssuer)
	}
	if cfg.AuthAudience == "" {
		return Config{}, fmt.Errorf("config: SWITCHYARD_AUTH_AUDIENCE must not be empty")
	}
	if cfg.Addr == "" {
		return Config{}, fmt.Errorf("config: SWITCHYARD_ADDR must not be empty")
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
