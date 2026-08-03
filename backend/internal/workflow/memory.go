package workflow

import (
	"context"
	"sort"
	"sync"
)

// MemoryStore keeps workflows in memory so the rules in this package can be
// exercised without a database. It is not meant for production.
//
// It has to reject everything PostgresStore rejects. A fake that is more
// permissive than the real store is how a bug ships green: the tests pass
// against the lenient one and the deployment fails against the strict one.
type MemoryStore struct {
	mu        sync.RWMutex
	workflows map[string]Workflow // keyed by workspaceID + "\x00" + id
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{workflows: make(map[string]Workflow)}
}

func workflowKey(workspaceID, id string) string { return workspaceID + "\x00" + id }

func (m *MemoryStore) Create(_ context.Context, w Workflow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workflows[workflowKey(w.WorkspaceID, w.ID)] = w
	return nil
}

// Get keys on the workspace as well as the id, so asking the wrong workspace
// for a real workflow is a miss rather than a hit — the same answer Postgres
// gives, because the workspace is in its WHERE clause too.
func (m *MemoryStore) Get(_ context.Context, workspaceID, id string) (Workflow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	w, ok := m.workflows[workflowKey(workspaceID, id)]
	if !ok {
		return Workflow{}, ErrNotFound
	}
	return w, nil
}

func (m *MemoryStore) List(_ context.Context, workspaceID string) ([]Workflow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var found []Workflow
	for _, w := range m.workflows {
		if w.WorkspaceID == workspaceID {
			found = append(found, w)
		}
	}
	// Map iteration is random; Postgres orders by name, so this does too or
	// the two stores disagree about something a caller can see.
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, nil
}

func (m *MemoryStore) Update(_ context.Context, w Workflow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := workflowKey(w.WorkspaceID, w.ID)
	if _, ok := m.workflows[key]; !ok {
		return ErrNotFound
	}
	m.workflows[key] = w
	return nil
}

func (m *MemoryStore) Delete(_ context.Context, workspaceID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := workflowKey(workspaceID, id)
	if _, ok := m.workflows[key]; !ok {
		return ErrNotFound
	}
	delete(m.workflows, key)
	return nil
}

var _ Store = (*MemoryStore)(nil)
