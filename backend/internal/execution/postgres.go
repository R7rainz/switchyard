package execution

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the real Store. The queries live here, beside the rules they
// serve; the database package owns the pool and the schema and nothing else.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// defaultListLimit bounds a listing that did not ask for one. An execution list
// grows forever, and the dashboard shows a page of it.
const defaultListLimit = 50

func (s *PostgresStore) Create(ctx context.Context, e Execution) error {
	graph, err := json.Marshal(e.Graph)
	if err != nil {
		return fmt.Errorf("execution: encoding graph: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`insert into "execution"
		   ("id", "workspaceId", "workflowId", "graph", "status", "trigger", "input", "startedBy", "createdAt", "retryOf", "idempotencyKey")
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		e.ID, e.WorkspaceID, nullString(e.WorkflowID), graph, string(e.Status),
		e.Trigger, nullJSON(e.Input), nullString(e.StartedBy), e.CreatedAt,
		nullString(e.RetryOf), nullString(e.IdempotencyKey))
	if err != nil {
		return fmt.Errorf("execution: creating: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetByIdempotencyKey(ctx context.Context, workspaceID, key string) (Execution, error) {
	row := s.pool.QueryRow(ctx, executionColumns+` where "workspaceId" = $1 and "idempotencyKey" = $2`, workspaceID, key)
	run, err := scanExecution(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Execution{}, ErrNotFound
	}
	if err != nil {
		return Execution{}, fmt.Errorf("execution: loading by idempotency key: %w", err)
	}
	return run, nil
}

// Get filters on the workspace as well as the id, so an execution id from
// another workspace is a miss rather than a leak.
func (s *PostgresStore) Get(ctx context.Context, workspaceID, id string) (Execution, error) {
	row := s.pool.QueryRow(ctx, executionColumns+` where "workspaceId" = $1 and "id" = $2`, workspaceID, id)

	run, err := scanExecution(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Execution{}, ErrNotFound
	}
	if err != nil {
		return Execution{}, fmt.Errorf("execution: loading: %w", err)
	}
	return run, nil
}

func (s *PostgresStore) List(ctx context.Context, workspaceID, workflowID string, limit int) ([]Execution, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}

	// One statement rather than two, with the workflow filter switched off by
	// passing null. Two near-identical queries is two places for the workspace
	// predicate to be forgotten from.
	rows, err := s.pool.Query(ctx,
		executionColumns+` where "workspaceId" = $1
		   and ($2::text is null or "workflowId" = $2)
		 order by "createdAt" desc, "id" desc
		 limit $3`,
		workspaceID, nullString(workflowID), limit)
	if err != nil {
		return nil, fmt.Errorf("execution: listing: %w", err)
	}
	defer rows.Close()

	var found []Execution
	for rows.Next() {
		run, err := scanExecution(rows)
		if err != nil {
			return nil, fmt.Errorf("execution: listing: %w", err)
		}
		found = append(found, run)
	}
	return found, rows.Err()
}

func (s *PostgresStore) Start(ctx context.Context, id string, at time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`update "execution" set "status" = $2, "startedAt" = $3 where "id" = $1`,
		id, string(StatusRunning), at)
	if err != nil {
		return fmt.Errorf("execution: starting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Finish writes a terminal status, and only over a status that is not already
// terminal. Without that guard a node finishing just as a cancel lands would
// overwrite CANCELLED with SUCCEEDED, and the run would report the opposite of
// what the user asked for.
func (s *PostgresStore) Finish(ctx context.Context, id string, status Status, message string, at time.Time) error {
	_, err := s.pool.Exec(ctx,
		`update "execution"
		 set "status" = $2, "error" = $3, "finishedAt" = $4
		 where "id" = $1 and "status" in ('PENDING', 'RUNNING')`,
		id, string(status), nullString(message), at)
	if err != nil {
		return fmt.Errorf("execution: finishing: %w", err)
	}
	return nil
}

func (s *PostgresStore) finishIfRunning(ctx context.Context, id string, status Status, message string, at time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`update "execution"
		 set "status" = $2, "error" = $3, "finishedAt" = $4
		 where "id" = $1 and "status" in ('PENDING', 'RUNNING')`,
		id, string(status), nullString(message), at)
	if err != nil {
		return false, fmt.Errorf("execution: finishing: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PostgresStore) SaveNodeRun(ctx context.Context, row NodeRun) error {
	_, err := s.pool.Exec(ctx,
		`insert into "execution_node"
		   ("id", "executionId", "nodeId", "status", "output", "error", "startedAt", "finishedAt")
		 values ($1, $2, $3, $4, $5, $6, $7, $8)
		 on conflict ("executionId", "nodeId") do update set
		   "status" = excluded."status", "output" = excluded."output",
		   "error" = excluded."error", "startedAt" = excluded."startedAt",
		   "finishedAt" = excluded."finishedAt"`,
		rand.Text(), row.ExecutionID, row.NodeID, string(row.Status),
		nullJSON(row.Output), nullString(row.Error), nullTime(row.StartedAt), nullTime(row.FinishedAt))
	if err != nil {
		return fmt.Errorf("execution: saving node run: %w", err)
	}
	return nil
}

// NodeRuns returns the rows in the order the nodes started, which is the order
// the execution viewer draws them. Rows never started sort last.
func (s *PostgresStore) NodeRuns(ctx context.Context, executionID string) ([]NodeRun, error) {
	rows, err := s.pool.Query(ctx,
		`select "executionId", "nodeId", "status", "output", "error", "startedAt", "finishedAt"
		 from "execution_node" where "executionId" = $1
		 order by "startedAt" asc nulls last, "nodeId"`,
		executionID)
	if err != nil {
		return nil, fmt.Errorf("execution: loading node runs: %w", err)
	}
	defer rows.Close()

	var found []NodeRun
	for rows.Next() {
		var (
			run        NodeRun
			status     string
			output     []byte
			message    *string
			startedAt  *time.Time
			finishedAt *time.Time
		)
		if err := rows.Scan(&run.ExecutionID, &run.NodeID, &status, &output, &message, &startedAt, &finishedAt); err != nil {
			return nil, fmt.Errorf("execution: loading node runs: %w", err)
		}
		run.Status = Status(status)
		run.Output = output
		if message != nil {
			run.Error = *message
		}
		if startedAt != nil {
			run.StartedAt = *startedAt
		}
		if finishedAt != nil {
			run.FinishedAt = *finishedAt
		}
		found = append(found, run)
	}
	return found, rows.Err()
}

// Reclaim fails everything left unfinished. It crosses workspaces, which is
// correct and is why it is not reachable from a request: it runs once at
// startup, on behalf of the process rather than a user.
func (s *PostgresStore) Reclaim(ctx context.Context, message string) (int, error) {
	tag, err := s.pool.Exec(ctx,
		`update "execution"
		 set "status" = 'FAILED', "error" = $1, "finishedAt" = now()
		 where "status" in ('PENDING', 'RUNNING')`,
		message)
	if err != nil {
		return 0, fmt.Errorf("execution: reclaiming: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

const executionColumns = `select "id", "workspaceId", "workflowId", "graph", "status",
	"trigger", "input", "error", "startedBy", "createdAt", "startedAt", "finishedAt", "retryOf", "idempotencyKey"
	from "execution"`

func scanExecution(row pgx.Row) (Execution, error) {
	var (
		run            Execution
		workflowID     *string
		graph          []byte
		status         string
		input          []byte
		message        *string
		startedBy      *string
		startedAt      *time.Time
		finishedAt     *time.Time
		retryOf        *string
		idempotencyKey *string
	)

	if err := row.Scan(&run.ID, &run.WorkspaceID, &workflowID, &graph, &status,
		&run.Trigger, &input, &message, &startedBy, &run.CreatedAt, &startedAt, &finishedAt,
		&retryOf, &idempotencyKey); err != nil {
		return Execution{}, err
	}
	if err := json.Unmarshal(graph, &run.Graph); err != nil {
		return Execution{}, fmt.Errorf("execution: decoding graph: %w", err)
	}

	run.Status = Status(status)
	run.Input = input
	if workflowID != nil {
		run.WorkflowID = *workflowID
	}
	if message != nil {
		run.Error = *message
	}
	if startedBy != nil {
		run.StartedBy = *startedBy
	}
	if startedAt != nil {
		run.StartedAt = *startedAt
	}
	if finishedAt != nil {
		run.FinishedAt = *finishedAt
	}
	if retryOf != nil {
		run.RetryOf = *retryOf
	}
	if idempotencyKey != nil {
		run.IdempotencyKey = *idempotencyKey
	}
	return run, nil
}

// The schema uses NULL where the domain uses a zero value, so "not yet" and
// "none" are one concept each rather than two.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nullJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

var _ Store = (*PostgresStore)(nil)
