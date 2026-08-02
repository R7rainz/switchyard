package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
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

	// CredentialKeys are the AES-256 master keys that encrypt stored
	// third-party secrets, by version. More than one is present only during a
	// rotation, where the retired key still has to open the rows the new one
	// has not rewritten yet.
	//
	// These are secret: log named fields, never a whole Config.
	CredentialKeys map[int][]byte

	// CredentialKeyVersion is the version new credentials are sealed under.
	CredentialKeyVersion int

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

	keys, current, err := credentialKeys(os.Getenv(credentialKeyEnv))
	if err != nil {
		return Config{}, err
	}
	cfg.CredentialKeys = keys
	cfg.CredentialKeyVersion = current

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

const (
	credentialKeyEnv = "SWITCHYARD_CREDENTIAL_KEY"

	// AES-256, matching credential.KeySize. The constant is repeated rather
	// than imported: config sits below every domain package and must not start
	// importing them.
	credentialKeySize = 32
)

// credentialKeys parses SWITCHYARD_CREDENTIAL_KEY, which holds one or more
// versioned master keys, newest first:
//
//	SWITCHYARD_CREDENTIAL_KEY="2:$(openssl rand -base64 32),1:<the old key>"
//
// The first entry encrypts new credentials; the rest exist only so rows written
// before a rotation still open. The version is explicit rather than positional
// because it is stored on every row — the database decides which key a given
// credential needs, so the numbers here have to be the ones it saw.
//
// Every failure is a startup error: a key that is missing, mistyped, or the
// wrong length would otherwise surface much later as credentials that cannot be
// read, which is a far worse time to find out.
func credentialKeys(raw string) (map[int][]byte, int, error) {
	if raw == "" {
		return nil, 0, fmt.Errorf("config: %s must be set to at least one \"version:base64key\" entry, where the key is %d random bytes", credentialKeyEnv, credentialKeySize)
	}

	keys := make(map[int][]byte)
	current := 0
	for i, entry := range strings.Split(raw, ",") {
		version, encoded, found := strings.Cut(strings.TrimSpace(entry), ":")
		if !found {
			return nil, 0, fmt.Errorf("config: %s entry %d is not in \"version:base64key\" form", credentialKeyEnv, i+1)
		}

		number, err := strconv.Atoi(version)
		if err != nil || number <= 0 {
			return nil, 0, fmt.Errorf("config: %s entry %d has key version %q, want a positive integer", credentialKeyEnv, i+1, version)
		}
		if _, duplicate := keys[number]; duplicate {
			return nil, 0, fmt.Errorf("config: %s lists key version %d twice", credentialKeyEnv, number)
		}

		// Nothing below echoes the value, only what is wrong with it.
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return nil, 0, fmt.Errorf("config: %s key version %d is not valid base64", credentialKeyEnv, number)
		}
		if len(key) != credentialKeySize {
			return nil, 0, fmt.Errorf("config: %s key version %d decodes to %d bytes, want %d", credentialKeyEnv, number, len(key), credentialKeySize)
		}

		keys[number] = key
		if i == 0 {
			current = number
		}
	}
	return keys, current, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
