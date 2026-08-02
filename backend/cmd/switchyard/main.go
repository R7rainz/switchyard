// Command switchyard runs the Switchyard API server.
package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"

	"github.com/R7rainz/switchyard/backend/internal/api"
	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/config"
	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/database"
	"github.com/R7rainz/switchyard/backend/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// The logger is not built yet, so this one goes out plainly.
		log.Fatal(err)
	}

	logger := newLogger(cfg)
	// Anything reaching for zerolog's package-level logger gets this one, so
	// no log line escapes in a different shape.
	zlog.Logger = logger

	verifier := auth.NewVerifier(cfg.AuthJWKSURL(), cfg.AuthIssuer, cfg.AuthAudience)

	// Nothing can be wired to the credential service until there is a store for
	// it, but the master keys are checked here anyway: a key AES will not take
	// should stop the process now, not the first time someone saves a token.
	if _, err := credential.NewKeyring(cfg.CredentialKeyVersion, cfg.CredentialKeys); err != nil {
		logger.Fatal().Err(err).Msg("credential keys unusable")
	}

	// Bound startup separately from the request timeouts below: an unreachable
	// database should fail the boot, not hang it.
	startupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.Connect(startupCtx, database.Options{
		URL:            cfg.DatabaseURL,
		MaxConns:       cfg.DatabaseMaxConns,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("database unavailable")
	}
	defer pool.Close()

	applied, err := database.Migrate(startupCtx, pool, migrations.FS)
	if err != nil {
		logger.Fatal().Err(err).Msg("migrations failed")
	}
	for _, migration := range applied {
		logger.Info().Int64("version", migration.Version).Str("name", migration.Name).Msg("migration applied")
	}
	if len(applied) == 0 {
		logger.Debug().Msg("schema already up to date")
	}

	// A binary older than the schema it is pointed at is the usual way a
	// rollback goes wrong quietly.
	if err := database.Verify(startupCtx, pool, migrations.FS); err != nil {
		logger.Fatal().Err(err).Msg("schema is ahead of this build")
	}

	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: api.NewRouter(verifier, logger),
		// Bound how long a connection can sit half-open holding a slot.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	logger.Info().
		Str("addr", cfg.Addr).
		Str("issuer", cfg.AuthIssuer).
		Str("audience", cfg.AuthAudience).
		Bool("development", cfg.Development).
		Int("credential_key_version", cfg.CredentialKeyVersion).
		Msg("starting switchyard")

	if err := server.ListenAndServe(); err != nil {
		logger.Error().Err(err).Msg("server stopped")
		os.Exit(1)
	}
}

// newLogger writes human-readable console output while developing and JSON
// everywhere else, since only one of those two audiences is a person.
func newLogger(cfg config.Config) zerolog.Logger {
	var out io.Writer = os.Stdout
	if cfg.Development {
		out = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.TimeOnly}
	}

	return zerolog.New(out).Level(cfg.LogLevel).With().Timestamp().Logger()
}
