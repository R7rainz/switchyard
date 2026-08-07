package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (s *PostgresStore) CreateWithVersion(ctx context.Context, w Workflow, version Version) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("workflow: beginning create transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	graph, err := json.Marshal(w.Graph)
	if err != nil {
		return fmt.Errorf("workflow: encoding graph: %w", err)
	}
	if _, err = tx.Exec(ctx, `insert into "workflow" ("id", "workspaceId", "name", "description", "graph", "createdBy", "createdAt", "updatedAt") values ($1, $2, $3, $4, $5, $6, $7, $8)`, w.ID, w.WorkspaceID, w.Name, w.Description, graph, nullString(w.CreatedBy), w.CreatedAt, w.UpdatedAt); err != nil {
		return fmt.Errorf("workflow: creating: %w", err)
	}
	if err = insertVersion(ctx, tx, version); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("workflow: committing create: %w", err)
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

func (s *PostgresStore) GetByID(ctx context.Context, id string) (Workflow, error) {
	row := s.pool.QueryRow(ctx, workflowColumns+` where "id" = $1`, id)
	w, err := scanWorkflow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, ErrNotFound
	}
	if err != nil {
		return Workflow{}, fmt.Errorf("workflow: loading public workflow: %w", err)
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

func (s *PostgresStore) UpdateWithVersion(ctx context.Context, w Workflow, version Version) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("workflow: beginning update transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `select "id" from "workflow" where "workspaceId" = $1 and "id" = $2 for update`, w.WorkspaceID, w.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("workflow: locking: %w", err)
	}
	graph, err := json.Marshal(w.Graph)
	if err != nil {
		return fmt.Errorf("workflow: encoding graph: %w", err)
	}
	if _, err = tx.Exec(ctx, `update "workflow" set "name" = $3, "description" = $4, "graph" = $5, "updatedAt" = $6 where "workspaceId" = $1 and "id" = $2`, w.WorkspaceID, w.ID, w.Name, w.Description, graph, w.UpdatedAt); err != nil {
		return fmt.Errorf("workflow: updating: %w", err)
	}
	if err = tx.QueryRow(ctx, `select coalesce(max("number"), 0) + 1 from "workflow_version" where "workspaceId" = $1 and "workflowId" = $2`, w.WorkspaceID, w.ID).Scan(&version.Number); err != nil {
		return fmt.Errorf("workflow: allocating version: %w", err)
	}
	if err = insertVersion(ctx, tx, version); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("workflow: committing update: %w", err)
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

func (s *PostgresStore) CreateVersion(ctx context.Context, version Version) error {
	return insertVersion(ctx, s.pool, version)
}

type queryExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertVersion(ctx context.Context, execer queryExecer, version Version) error {
	graph, err := json.Marshal(version.Graph)
	if err != nil {
		return fmt.Errorf("workflow: encoding version graph: %w", err)
	}
	_, err = execer.Exec(ctx,
		`insert into "workflow_version"
		   ("id", "workspaceId", "workflowId", "number", "name", "description", "graph", "createdBy", "createdAt")
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		version.ID, version.WorkspaceID, version.WorkflowID, version.Number, version.Name,
		version.Description, graph, nullString(version.CreatedBy), version.CreatedAt)
	if err != nil {
		return fmt.Errorf("workflow: creating version: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListVersions(ctx context.Context, workspaceID, workflowID string) ([]Version, error) {
	rows, err := s.pool.Query(ctx,
		`select "id", "workspaceId", "workflowId", "number", "name", "description", "graph", "createdBy", "createdAt"
		 from "workflow_version" where "workspaceId" = $1 and "workflowId" = $2 order by "number"`,
		workspaceID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow: listing versions: %w", err)
	}
	defer rows.Close()
	var versions []Version
	for rows.Next() {
		version, err := scanVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("workflow: listing versions: %w", err)
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *PostgresStore) GetVersion(ctx context.Context, workspaceID, workflowID string, number int) (Version, error) {
	row := s.pool.QueryRow(ctx,
		`select "id", "workspaceId", "workflowId", "number", "name", "description", "graph", "createdBy", "createdAt"
		 from "workflow_version" where "workspaceId" = $1 and "workflowId" = $2 and "number" = $3`,
		workspaceID, workflowID, number)
	version, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, fmt.Errorf("workflow: loading version: %w", err)
	}
	return version, nil
}

func (s *PostgresStore) CreateTemplate(ctx context.Context, template Template) error {
	graph, err := json.Marshal(template.Graph)
	if err != nil {
		return fmt.Errorf("workflow: encoding template graph: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`insert into "workflow_template"
		   ("id", "workspaceId", "name", "description", "graph", "createdBy", "createdAt")
		 values ($1, $2, $3, $4, $5, $6, $7)`,
		template.ID, template.WorkspaceID, template.Name, template.Description, graph,
		nullString(template.CreatedBy), template.CreatedAt)
	if err != nil {
		return fmt.Errorf("workflow: creating template: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListTemplates(ctx context.Context, workspaceID string) ([]Template, error) {
	rows, err := s.pool.Query(ctx,
		`select "id", "workspaceId", "name", "description", "graph", "createdBy", "createdAt"
		 from "workflow_template" where "workspaceId" = $1 order by "name"`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workflow: listing templates: %w", err)
	}
	defer rows.Close()
	var templates []Template
	for rows.Next() {
		template, err := scanTemplate(rows)
		if err != nil {
			return nil, fmt.Errorf("workflow: listing templates: %w", err)
		}
		templates = append(templates, template)
	}
	return templates, rows.Err()
}

func (s *PostgresStore) GetTemplate(ctx context.Context, workspaceID, id string) (Template, error) {
	row := s.pool.QueryRow(ctx,
		`select "id", "workspaceId", "name", "description", "graph", "createdBy", "createdAt"
		 from "workflow_template" where "workspaceId" = $1 and "id" = $2`, workspaceID, id)
	template, err := scanTemplate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("workflow: loading template: %w", err)
	}
	return template, nil
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

func scanVersion(row pgx.Row) (Version, error) {
	var (
		version   Version
		graph     []byte
		createdBy *string
	)
	if err := row.Scan(&version.ID, &version.WorkspaceID, &version.WorkflowID, &version.Number,
		&version.Name, &version.Description, &graph, &createdBy, &version.CreatedAt); err != nil {
		return Version{}, err
	}
	if err := json.Unmarshal(graph, &version.Graph); err != nil {
		return Version{}, fmt.Errorf("workflow: decoding version graph: %w", err)
	}
	if createdBy != nil {
		version.CreatedBy = *createdBy
	}
	return version, nil
}

func scanTemplate(row pgx.Row) (Template, error) {
	var (
		template  Template
		graph     []byte
		createdBy *string
	)
	if err := row.Scan(&template.ID, &template.WorkspaceID, &template.Name, &template.Description,
		&graph, &createdBy, &template.CreatedAt); err != nil {
		return Template{}, err
	}
	if err := json.Unmarshal(graph, &template.Graph); err != nil {
		return Template{}, fmt.Errorf("workflow: decoding template graph: %w", err)
	}
	if createdBy != nil {
		template.CreatedBy = *createdBy
	}
	return template, nil
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
