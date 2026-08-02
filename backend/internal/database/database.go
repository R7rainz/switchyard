package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the connection pool the whole process shares. It is an alias rather
// than a wrapper so domain packages get pgx's own API — wrapping it would mean
// re-exporting every method they need, one at a time, forever.
type Pool = pgxpool.Pool

// Options configures the pool. Everything has a working default except the URL.
type Options struct {
	// URL is a libpq connection string or postgres:// URL.
	URL string

	// MaxConns bounds the pool. Postgres itself is the scarce resource here:
	// each connection is a backend process, so a pool larger than the server's
	// max_connections divided by the number of instances only turns a queue
	// inside this process into a connection error from the server.
	MaxConns int32

	// ConnectTimeout bounds the initial connection, so a wrong host fails
	// startup quickly rather than hanging.
	ConnectTimeout time.Duration
}

// Connect opens the pool and verifies it can actually reach the database.
//
// pgxpool connects lazily, so without the ping a bad URL or an unreachable
// server would look like a healthy start and fail on the first request
// instead.
func Connect(ctx context.Context, opts Options) (*Pool, error) {
	if opts.URL == "" {
		return nil, fmt.Errorf("database: no connection URL")
	}

	config, err := pgxpool.ParseConfig(opts.URL)
	if err != nil {
		// The URL carries the password, so the parse error goes in without it.
		return nil, fmt.Errorf("database: connection URL is not valid")
	}

	if opts.MaxConns > 0 {
		config.MaxConns = opts.MaxConns
	}
	if opts.ConnectTimeout > 0 {
		config.ConnConfig.ConnectTimeout = opts.ConnectTimeout
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("database: opening pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: cannot reach the server: %w", err)
	}
	return pool, nil
}
