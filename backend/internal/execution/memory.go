package execution

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryStore keeps executions in memory so the engine can be tested without a
// database. It has to refuse everything PostgresStore refuses; a fake that is
// more permissive is how a bug ships green.
type MemoryStore struct {
	mu    sync.RWMutex
	runs  map[string]Execution // keyed workspaceID + "\x00" + id
	nodes map[string][]NodeRun // keyed by execution id
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		runs:  make(map[string]Execution),
		nodes: make(map[string][]NodeRun),
	}
}

func runKey(workspaceID, id string) string { return workspaceID + "\x00" + id }

func (m *MemoryStore) Create(_ context.Context, e Execution) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[runKey(e.WorkspaceID, e.ID)] = e
	return nil
}

func (m *MemoryStore) Get(_ context.Context, workspaceID, id string) (Execution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, ok := m.runs[runKey(workspaceID, id)]
	if !ok {
		return Execution{}, ErrNotFound
	}
	return run, nil
}

func (m *MemoryStore) List(_ context.Context, workspaceID, workflowID string, limit int) ([]Execution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var found []Execution
	for _, run := range m.runs {
		if run.WorkspaceID != workspaceID {
			continue
		}
		if workflowID != "" && run.WorkflowID != workflowID {
			continue
		}
		found = append(found, run)
	}

	// Newest first, matching the SQL. Ties break on id so the order is stable
	// when a test creates several runs inside one clock tick.
	sort.Slice(found, func(i, j int) bool {
		if found[i].CreatedAt.Equal(found[j].CreatedAt) {
			return found[i].ID > found[j].ID
		}
		return found[i].CreatedAt.After(found[j].CreatedAt)
	})
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	return found, nil
}

// byID finds a run without knowing its workspace, for the engine's own writes.
// The caller already holds the lock.
func (m *MemoryStore) byID(id string) (string, bool) {
	for key, run := range m.runs {
		if run.ID == id {
			return key, true
		}
	}
	return "", false
}

func (m *MemoryStore) Start(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, ok := m.byID(id)
	if !ok {
		return ErrNotFound
	}
	run := m.runs[key]
	run.Status = StatusRunning
	run.StartedAt = at
	m.runs[key] = run
	return nil
}

func (m *MemoryStore) Finish(_ context.Context, id string, status Status, message string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, ok := m.byID(id)
	if !ok {
		return ErrNotFound
	}
	run := m.runs[key]
	// Already finished: leave it. A node completing just as a cancel lands
	// would otherwise overwrite CANCELLED with SUCCEEDED, and the run would
	// report the opposite of what the user asked for. The SQL has the same
	// guard in its WHERE clause.
	if run.Status.Done() {
		return nil
	}
	run.Status = status
	run.Error = message
	run.FinishedAt = at
	m.runs[key] = run
	return nil
}

func (m *MemoryStore) SaveNodeRun(_ context.Context, row NodeRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rows := m.nodes[row.ExecutionID]
	for i, existing := range rows {
		if existing.NodeID == row.NodeID {
			rows[i] = row
			return nil
		}
	}
	m.nodes[row.ExecutionID] = append(rows, row)
	return nil
}

func (m *MemoryStore) NodeRuns(_ context.Context, executionID string) ([]NodeRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows := make([]NodeRun, len(m.nodes[executionID]))
	copy(rows, m.nodes[executionID])
	return rows, nil
}

func (m *MemoryStore) Reclaim(_ context.Context, message string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	reclaimed := 0
	for key, run := range m.runs {
		if run.Status == StatusPending || run.Status == StatusRunning {
			run.Status = StatusFailed
			run.Error = message
			m.runs[key] = run
			reclaimed++
		}
	}
	return reclaimed, nil
}

var _ Store = (*MemoryStore)(nil)
