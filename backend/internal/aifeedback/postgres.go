package aifeedback

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (s *PostgresStore) Create(ctx context.Context, record Record) error {
	_, err := s.pool.Exec(ctx,
		`insert into "ai_feedback"
		 ("id", "workspaceId", "userId", "prompt", "outcome", "generatedName", "generatedDescription", "generatedGraph", "finalGraph", "createdAt")
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		record.ID, record.WorkspaceID, record.UserID, record.Prompt, record.Outcome,
		record.GeneratedName, record.GeneratedDescription, record.GeneratedGraph, nullJSON(record.FinalGraph), record.CreatedAt)
	if err != nil {
		return fmt.Errorf("ai feedback: creating: %w", err)
	}
	return nil
}

func nullJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

var _ Store = (*PostgresStore)(nil)
