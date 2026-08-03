package credential

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the real Store. It moves ciphertext and never sees a key —
// the Keyring stays on the far side of the Service.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore returns a Store backed by pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// Save inserts, or replaces the secret already held for this workspace,
// provider, and name.
//
// The conflict target is the unique triple, so a replacement keeps the row's
// original id and createdAt. That matters beyond tidiness: the id is what an
// operator sees in a rotation failure, and it should not change every time
// someone updates a key.
func (s *PostgresStore) Save(ctx context.Context, record Record) error {
	_, err := s.pool.Exec(ctx,
		`insert into "credentials"
		   ("id", "workspaceId", "provider", "name", "ciphertext", "nonce", "keyVersion", "createdAt", "updatedAt")
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 on conflict ("workspaceId", "provider", "name") do update set
		   "ciphertext" = excluded."ciphertext",
		   "nonce"      = excluded."nonce",
		   "keyVersion" = excluded."keyVersion",
		   "updatedAt"  = excluded."updatedAt"`,
		record.ID, record.WorkspaceID, record.Provider, record.Name,
		record.Ciphertext, record.Nonce, record.KeyVersion, record.CreatedAt, record.UpdatedAt)
	if err != nil {
		// The secret is in record.Ciphertext and stays there; only the failure
		// travels.
		return fmt.Errorf("credential: saving: %w", err)
	}
	return nil
}

// Get loads one credential. The workspace is in the WHERE clause, never a
// filter applied afterwards, so another workspace's row cannot come back.
func (s *PostgresStore) Get(ctx context.Context, workspaceID, provider, name string) (Record, error) {
	row := s.pool.QueryRow(ctx, credentialColumns+
		` where "workspaceId" = $1 and "provider" = $2 and "name" = $3`,
		workspaceID, provider, name)

	record, err := scanCredential(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("credential: loading: %w", err)
	}
	return record, nil
}

func (s *PostgresStore) List(ctx context.Context, workspaceID string) ([]Record, error) {
	rows, err := s.pool.Query(ctx, credentialColumns+
		` where "workspaceId" = $1 order by "provider", "name"`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("credential: listing: %w", err)
	}
	defer rows.Close()

	var found []Record
	for rows.Next() {
		record, err := scanCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("credential: listing: %w", err)
		}
		found = append(found, record)
	}
	return found, rows.Err()
}

func (s *PostgresStore) Delete(ctx context.Context, workspaceID, provider, name string) error {
	tag, err := s.pool.Exec(ctx,
		`delete from "credentials" where "workspaceId" = $1 and "provider" = $2 and "name" = $3`,
		workspaceID, provider, name)
	if err != nil {
		return fmt.Errorf("credential: deleting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Stale returns records still sealed under an older key.
//
// The ordering is by id so repeated calls make progress rather than returning
// the same batch: rotation rewrites the key version, so a row that has been
// done drops out of this result entirely, and a stable order means the next
// call picks up where the last one stopped.
func (s *PostgresStore) Stale(ctx context.Context, keyVersion, limit int) ([]Record, error) {
	if limit <= 0 {
		return nil, errors.New("credential: rotation limit must be positive")
	}

	rows, err := s.pool.Query(ctx, credentialColumns+
		` where "keyVersion" <> $1 order by "id" limit $2`, keyVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("credential: listing stale: %w", err)
	}
	defer rows.Close()

	var found []Record
	for rows.Next() {
		record, err := scanCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("credential: listing stale: %w", err)
		}
		found = append(found, record)
	}
	return found, rows.Err()
}

const credentialColumns = `select "id", "workspaceId", "provider", "name",
	"ciphertext", "nonce", "keyVersion", "createdAt", "updatedAt" from "credentials"`

func scanCredential(row pgx.Row) (Record, error) {
	var record Record
	err := row.Scan(&record.ID, &record.WorkspaceID, &record.Provider, &record.Name,
		&record.Ciphertext, &record.Nonce, &record.KeyVersion, &record.CreatedAt, &record.UpdatedAt)
	return record, err
}

var _ Store = (*PostgresStore)(nil)
