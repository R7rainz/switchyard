// Command switchyard runs the Switchyard API server.
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/api"
	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	verifier := auth.NewVerifier(cfg.AuthJWKSURL(), cfg.AuthIssuer, cfg.AuthAudience)

	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: api.NewRouter(verifier),
		// Bound how long a connection can sit half-open holding a slot.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	log.Printf("switchyard listening on %s, trusting tokens from %s", cfg.Addr, cfg.AuthIssuer)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
