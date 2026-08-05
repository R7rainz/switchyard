package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the real Store. The queries live here, next to the rules
// they serve, rather than in the database package — that one owns the pool and
// the schema and nothing else.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore returns a Store backed by pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Create(ctx context.Context, w Workflow) error {
	graph, err := json.Marshal(w.Graph)
	if err != nil {
		return fmt.Errorf("workflow: encoding graph: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`insert into "workflow"
		   ("id", "workspaceId", "name", "description", "graph", "createdBy", "createdAt", "updatedAt")
		 values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		w.ID, w.WorkspaceID, w.Name, w.Description, graph, nullString(w.CreatedBy), w.CreatedAt, w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("workflow: creating: %w", err)
	}
	return nil
}

// Get filters on the workspace as well as the id. The id alone would be enough
// to find the row, which is the danger: permission was checked against the
// workspace in the URL, so dropping this predicate would serve one workspace's
// workflow to another's admin.
func (s *PostgresStore) Get(ctx context.Context, workspaceID, id string) (Workflow, error) {
	row := s.pool.QueryRow(ctx,
		workflowColumns+` where "workspaceId" = $1 and "id" = $2`,
		workspaceID, id)

	w, err := scanWorkflow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, ErrNotFound
	}
	if err != nil {
		return Workflow{}, fmt.Errorf("workflow: loading: %w", err)
	}
	return w, nil
}

func (s *PostgresStore) List(ctx context.Context, workspaceID string) ([]Workflow, error) {
	rows, err := s.pool.Query(ctx,
		workflowColumns+` where "workspaceId" = $1 order by "name"`,
		workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workflow: listing: %w", err)
	}
	defer rows.Close()

	var found []Workflow
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, fmt.Errorf("workflow: listing: %w", err)
		}
		found = append(found, w)
	}
	return found, rows.Err()
}

func (s *PostgresStore) ListAll(ctx context.Context) ([]Workflow, error) {
	rows, err := s.pool.Query(ctx, workflowColumns+` order by "id"`)
	if err != nil {
		return nil, fmt.Errorf("workflow: listing all: %w", err)
	}
	defer rows.Close()
	var found []Workflow
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, fmt.Errorf("workflow: listing all: %w", err)
		}
		found = append(found, w)
	}
	return found, rows.Err()
}

// Update rewrites everything the caller may change. The workspace and the id
// are the key, never the payload: a patch cannot move a workflow to another
// workspace, because there is nowhere in this statement to say so.
func (s *PostgresStore) Update(ctx context.Context, w Workflow) error {
	graph, err := json.Marshal(w.Graph)
	if err != nil {
		return fmt.Errorf("workflow: encoding graph: %w", err)
	}

	tag, err := s.pool.Exec(ctx,
		`update "workflow"
		 set "name" = $3, "description" = $4, "graph" = $5, "updatedAt" = $6
		 where "workspaceId" = $1 and "id" = $2`,
		w.WorkspaceID, w.ID, w.Name, w.Description, graph, w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("workflow: updating: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) Delete(ctx context.Context, workspaceID, id string) error {
	tag, err := s.pool.Exec(ctx,
		`delete from "workflow" where "workspaceId" = $1 and "id" = $2`,
		workspaceID, id)
	if err != nil {
		return fmt.Errorf("workflow: deleting: %w", err)
	}
	// Deleting nothing is reported rather than swallowed: the caller named a
	// specific workflow, and silence would look like success.
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const workflowColumns = `select "id", "workspaceId", "name", "description", "graph",
	"createdBy", "createdAt", "updatedAt" from "workflow"`

func scanWorkflow(row pgx.Row) (Workflow, error) {
	var (
		w         Workflow
		graph     []byte
		createdBy *string
	)

	if err := row.Scan(&w.ID, &w.WorkspaceID, &w.Name, &w.Description, &graph,
		&createdBy, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return Workflow{}, err
	}

	if err := json.Unmarshal(graph, &w.Graph); err != nil {
		return Workflow{}, fmt.Errorf("workflow: decoding graph: %w", err)
	}
	if createdBy != nil {
		w.CreatedBy = *createdBy
	}
	return w, nil
}

// The schema allows a null createdBy — the user may be gone — while the domain
// uses the empty string, so "nobody" is one concept rather than two.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// PostgresStore and MemoryStore must stay interchangeable; the tests run the
// same expectations against both.
var _ Store = (*PostgresStore)(nil)
