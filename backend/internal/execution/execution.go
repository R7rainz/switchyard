package execution

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

var (
	// ErrNotFound means no execution with that id exists in that workspace.
	ErrNotFound = errors.New("execution: not found")

	// ErrNotRunning means a cancel arrived after the run had already finished.
	ErrNotRunning = errors.New("execution: already finished")

	// ErrNotRetryable means a retry was requested for a run that is still
	// active or already succeeded. Retrying success would duplicate side
	// effects, so recovery is deliberately limited to failed/cancelled runs.
	ErrNotRetryable = errors.New("execution: run cannot be retried")

	// ErrIdempotencyConflict means a key was reused for a different request.
	ErrIdempotencyConflict = errors.New("execution: idempotency key was reused")

	// ErrInvalidIdempotencyKey means the caller supplied an oversized key.
	ErrInvalidIdempotencyKey = errors.New("execution: invalid idempotency key")
)

// Status is where a run, or one node of it, has got to.
type Status string

const (
	StatusPending   Status = "PENDING"
	StatusRunning   Status = "RUNNING"
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
	StatusCancelled Status = "CANCELLED"

	// StatusSkipped is only ever a node's. A node on the branch that was not
	// taken did not fail and is not waiting — the viewer has to be able to say
	// so, or an untaken branch looks like a run that hung.
	StatusSkipped Status = "SKIPPED"
)

// Done reports whether this status is final.
func (s Status) Done() bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusCancelled, StatusSkipped:
		return true
	}
	return false
}

// Execution is one run of one workflow.
//
// Graph is a copy taken when the run started, not a reference to the workflow.
// That is what makes a finished execution mean something: editing the workflow
// afterwards cannot change what this run appears to have done. It is also why
// there is no version table — this snapshot answers the question versioning
// would have been asked.
type Execution struct {
	ID             string
	WorkspaceID    string
	WorkflowID     string
	Graph          workflow.Graph
	Status         Status
	Trigger        string
	Input          json.RawMessage
	Error          string
	StartedBy      string
	CreatedAt      time.Time
	StartedAt      time.Time
	FinishedAt     time.Time
	RetryOf        string
	IdempotencyKey string
}

// NodeRun is what one node did during one execution.
type NodeRun struct {
	ExecutionID string
	NodeID      string
	Status      Status
	Output      json.RawMessage
	Error       string
	StartedAt   time.Time
	FinishedAt  time.Time
}

// Store is the persistence this package needs, declared here where it is
// consumed. As everywhere else, the workspace is part of every lookup rather
// than an optional filter: permission was checked against the workspace in the
// URL, so a query by execution id alone would cross workspaces.
type Store interface {
	Create(ctx context.Context, e Execution) error
	Get(ctx context.Context, workspaceID, id string) (Execution, error)
	GetByIdempotencyKey(ctx context.Context, workspaceID, key string) (Execution, error)

	// List returns a workspace's executions newest first. An empty workflowID
	// means all of them.
	List(ctx context.Context, workspaceID, workflowID string, limit int) ([]Execution, error)

	// Finish records a terminal status. It takes the id alone because the
	// engine already holds an execution it created; nothing external calls it.
	Finish(ctx context.Context, id string, status Status, message string, at time.Time) error

	// Start moves an execution to RUNNING.
	Start(ctx context.Context, id string, at time.Time) error

	// SaveNodeRun inserts or replaces the row for one node of one run.
	SaveNodeRun(ctx context.Context, run NodeRun) error
	NodeRuns(ctx context.Context, executionID string) ([]NodeRun, error)

	// Reclaim fails every execution left PENDING or RUNNING, returning how
	// many. A process that dies mid-run leaves rows nothing will ever finish,
	// and a run that shows RUNNING forever is worse than one that shows failed:
	// the first looks like the engine is stuck, the second is the truth.
	Reclaim(ctx context.Context, message string) (int, error)
}

// Workflows is the part of the workflow package this one needs. Declared here
// so the engine can be tested without a workflow store.
type Workflows interface {
	Get(ctx context.Context, workspaceID, id string) (workflow.Workflow, error)
}

// Service starts runs and reports on them. The walking itself is in engine.go.
type Service struct {
	store     Store
	workflows Workflows
	runners   Registry
	now       func() time.Time
	newID     func() string

	// nodeTimeout bounds a single node. Without it one wedged HTTP call holds
	// an execution open forever, and "running" stops meaning anything.
	nodeTimeout time.Duration

	// runTimeout bounds the whole execution, since a graph of well-behaved
	// nodes can still add up to something nobody meant to start.
	runTimeout time.Duration

	// events is where progress is announced. Nil when nobody is listening.
	events Publisher

	// live tracks in-flight runs so Cancel has something to cancel. It is not
	// durable on purpose: a run only exists in this process, so a restart is
	// handled by Reclaim rather than by remembering anything here.
	live *liveRuns
}

// Options are the knobs with sensible defaults.
type Options struct {
	NodeTimeout time.Duration
	RunTimeout  time.Duration

	// Events receives what the engine is doing as it happens. Optional: a nil
	// Publisher means nobody is listening, which is the correct state for a
	// test and for a process nothing has connected to.
	Events Publisher
}

const (
	defaultNodeTimeout = 60 * time.Second
	defaultRunTimeout  = 15 * time.Minute
)

// NewService returns a Service that runs graphs with the given runners.
func NewService(store Store, workflows Workflows, runners Registry, opts Options) *Service {
	if opts.NodeTimeout <= 0 {
		opts.NodeTimeout = defaultNodeTimeout
	}
	if opts.RunTimeout <= 0 {
		opts.RunTimeout = defaultRunTimeout
	}
	return &Service{
		store:       store,
		workflows:   workflows,
		runners:     runners,
		now:         time.Now,
		newID:       func() string { return rand.Text() },
		nodeTimeout: opts.NodeTimeout,
		runTimeout:  opts.RunTimeout,
		events:      opts.Events,
		live:        newLiveRuns(),
	}
}

// Start validates that the workflow can run, snapshots it, and begins.
//
// It returns as soon as the execution row exists, because a workflow calls
// external services and can take minutes — far longer than a request should be
// held open. The caller gets an id and watches it.
func (s *Service) Start(ctx context.Context, workspaceID, workflowID, userID, trigger string, input json.RawMessage) (Execution, error) {
	return s.start(ctx, workspaceID, workflowID, userID, trigger, input, "")
}

// StartWithIdempotencyKey is Start with a caller-owned key. Repeating the same
// request returns the original row instead of launching another side effect.
func (s *Service) StartWithIdempotencyKey(ctx context.Context, workspaceID, workflowID, userID, trigger string, input json.RawMessage, key string) (Execution, error) {
	return s.start(ctx, workspaceID, workflowID, userID, trigger, input, key)
}

func (s *Service) start(ctx context.Context, workspaceID, workflowID, userID, trigger string, input json.RawMessage, key string) (Execution, error) {
	key, err := normalizeIdempotencyKey(key)
	if err != nil {
		return Execution{}, err
	}
	if trigger == "" {
		trigger = TriggerManual
	}
	// Resolve an existing key before loading the workflow. A client retry after
	// a workflow was deleted must still receive the original accepted run.
	if key != "" {
		existing, lookupErr := s.store.GetByIdempotencyKey(ctx, workspaceID, key)
		if lookupErr == nil {
			candidate := Execution{WorkspaceID: workspaceID, WorkflowID: workflowID, Trigger: trigger, Input: input, StartedBy: userID}
			if sameRequest(existing, candidate) {
				return existing, nil
			}
			return Execution{}, ErrIdempotencyConflict
		}
		if !errors.Is(lookupErr, ErrNotFound) {
			return Execution{}, lookupErr
		}
	}
	wf, err := s.workflows.Get(ctx, workspaceID, workflowID)
	if err != nil {
		return Execution{}, err
	}

	// This is where Runnable earns the split. The graph saved fine as a draft;
	// starting it is the moment it has to be a workflow that can actually
	// execute, and the caller gets told which part is not ready.
	if err := wf.Graph.Runnable(); err != nil {
		return Execution{}, err
	}
	now := s.now()
	run := Execution{
		ID:             s.newID(),
		WorkspaceID:    workspaceID,
		WorkflowID:     wf.ID,
		Graph:          wf.Graph,
		Status:         StatusPending,
		Trigger:        trigger,
		Input:          input,
		StartedBy:      userID,
		CreatedAt:      now,
		IdempotencyKey: key,
	}
	return s.persistAndLaunch(ctx, run)
}

// Retry starts a new execution from a failed or cancelled run's immutable
// snapshot. It does not consult the current workflow: recovery should still
// work after the workflow was edited or deleted.
func (s *Service) Retry(ctx context.Context, workspaceID, id, userID, key string) (Execution, error) {
	key, err := normalizeIdempotencyKey(key)
	if err != nil {
		return Execution{}, err
	}
	original, err := s.store.Get(ctx, workspaceID, id)
	if err != nil {
		return Execution{}, err
	}
	if original.Status != StatusFailed && original.Status != StatusCancelled {
		return Execution{}, ErrNotRetryable
	}
	if err := original.Graph.Runnable(); err != nil {
		return Execution{}, fmt.Errorf("execution: retry snapshot is not runnable: %w", err)
	}

	run := Execution{
		ID:             s.newID(),
		WorkspaceID:    workspaceID,
		WorkflowID:     original.WorkflowID,
		Graph:          original.Graph,
		Status:         StatusPending,
		Trigger:        TriggerRetry,
		Input:          original.Input,
		StartedBy:      userID,
		CreatedAt:      s.now(),
		RetryOf:        original.ID,
		IdempotencyKey: key,
	}
	return s.persistAndLaunch(ctx, run)
}

func (s *Service) persistAndLaunch(ctx context.Context, run Execution) (Execution, error) {
	if run.IdempotencyKey != "" {
		existing, err := s.store.GetByIdempotencyKey(ctx, run.WorkspaceID, run.IdempotencyKey)
		switch {
		case err == nil:
			if sameRequest(existing, run) {
				return existing, nil
			}
			return Execution{}, ErrIdempotencyConflict
		case !errors.Is(err, ErrNotFound):
			return Execution{}, err
		}
	}

	if err := s.store.Create(ctx, run); err != nil {
		// The database unique index wins a race between two first requests. Read
		// the winner and apply the same request comparison as the fast path.
		if run.IdempotencyKey != "" {
			if existing, lookupErr := s.store.GetByIdempotencyKey(ctx, run.WorkspaceID, run.IdempotencyKey); lookupErr == nil {
				if sameRequest(existing, run) {
					return existing, nil
				}
				return Execution{}, ErrIdempotencyConflict
			}
		}
		return Execution{}, err
	}

	s.launch(ctx, run)
	return run, nil
}

func normalizeIdempotencyKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if len(key) > 200 {
		return "", ErrInvalidIdempotencyKey
	}
	return key, nil
}

func sameRequest(a, b Execution) bool {
	return a.WorkspaceID == b.WorkspaceID && a.WorkflowID == b.WorkflowID &&
		a.RetryOf == b.RetryOf && a.Trigger == b.Trigger && a.StartedBy == b.StartedBy &&
		bytes.Equal(compactInput(a.Input), compactInput(b.Input))
}

func compactInput(input json.RawMessage) []byte {
	if len(input) == 0 {
		return nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, input); err != nil {
		return input
	}
	return compact.Bytes()
}

// Get returns one execution together with what each node did.
func (s *Service) Get(ctx context.Context, workspaceID, id string) (Execution, []NodeRun, error) {
	run, err := s.store.Get(ctx, workspaceID, id)
	if err != nil {
		return Execution{}, nil, err
	}
	runs, err := s.store.NodeRuns(ctx, run.ID)
	if err != nil {
		return Execution{}, nil, err
	}
	return run, runs, nil
}

// List returns a workspace's executions, newest first.
func (s *Service) List(ctx context.Context, workspaceID, workflowID string, limit int) ([]Execution, error) {
	return s.store.List(ctx, workspaceID, workflowID, limit)
}

// Cancel stops a run that is still going.
//
// It only reaches runs in this process. That is honest rather than limiting:
// a single binary is the whole deployment, and the day it is not, cancellation
// becomes a message rather than a map lookup.
func (s *Service) Cancel(ctx context.Context, workspaceID, id string) error {
	run, err := s.store.Get(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if run.Status.Done() {
		return ErrNotRunning
	}
	if !s.live.cancel(id) {
		// The row says running but nothing here is running it, which means the
		// process that was died. Finish it rather than leaving a row that never
		// resolves.
		if err := s.store.Finish(ctx, id, StatusCancelled, "cancelled", s.now()); err != nil {
			return err
		}
		// Announced here because no engine will: this run belongs to no
		// goroutine, so nothing else is going to tell a watcher it is over, and
		// they would sit on RUNNING for a run the database has cancelled.
		s.publish(Event{Type: EventExecution, ExecutionID: id, Status: StatusCancelled, Error: "cancelled"})
		return nil
	}
	return nil
}

// Reclaim fails runs left behind by a process that died, and belongs at
// startup, before the server accepts requests.
func (s *Service) Reclaim(ctx context.Context) (int, error) {
	return s.store.Reclaim(ctx, "interrupted: the server restarted while this run was in progress")
}
