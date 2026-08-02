package workspace

import (
	"context"
	"sync"
)

// MemoryStore keeps workspaces in memory. It exists so the rules in this
// package are testable before internal/database does, and so the API layer has
// something to run against; it is not meant for production.
type MemoryStore struct {
	mu         sync.RWMutex
	workspaces map[string]Workspace
	members    map[string]Member // keyed workspaceID + "\x00" + userID
	invites    map[string]Invite // keyed by invite id
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		workspaces: make(map[string]Workspace),
		members:    make(map[string]Member),
		invites:    make(map[string]Invite),
	}
}

func memberKey(workspaceID, userID string) string {
	return workspaceID + "\x00" + userID
}

func (m *MemoryStore) CreateWorkspace(_ context.Context, w Workspace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workspaces[w.ID] = w
	return nil
}

func (m *MemoryStore) GetWorkspace(_ context.Context, id string) (Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	w, ok := m.workspaces[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	return w, nil
}

func (m *MemoryStore) ListWorkspacesForUser(_ context.Context, userID string) ([]Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var found []Workspace
	for _, member := range m.members {
		if member.UserID != userID {
			continue
		}
		if w, ok := m.workspaces[member.WorkspaceID]; ok {
			found = append(found, w)
		}
	}
	return found, nil
}

func (m *MemoryStore) PutMember(_ context.Context, member Member) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.members[memberKey(member.WorkspaceID, member.UserID)] = member
	return nil
}

func (m *MemoryStore) GetMember(_ context.Context, workspaceID, userID string) (Member, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	member, ok := m.members[memberKey(workspaceID, userID)]
	if !ok {
		return Member{}, ErrNotFound
	}
	return member, nil
}

func (m *MemoryStore) ListMembers(_ context.Context, workspaceID string) ([]Member, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var found []Member
	for _, member := range m.members {
		if member.WorkspaceID == workspaceID {
			found = append(found, member)
		}
	}
	return found, nil
}

func (m *MemoryStore) DeleteMember(_ context.Context, workspaceID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := memberKey(workspaceID, userID)
	if _, ok := m.members[key]; !ok {
		return ErrNotFound
	}
	delete(m.members, key)
	return nil
}

func (m *MemoryStore) CreateInvite(_ context.Context, i Invite) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invites[i.ID] = i
	return nil
}

func (m *MemoryStore) GetInviteByTokenHash(_ context.Context, tokenHash string) (Invite, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, invite := range m.invites {
		if invite.TokenHash == tokenHash {
			return invite, nil
		}
	}
	return Invite{}, ErrNotFound
}

func (m *MemoryStore) ListInvites(_ context.Context, workspaceID string) ([]Invite, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var found []Invite
	for _, invite := range m.invites {
		if invite.WorkspaceID == workspaceID {
			found = append(found, invite)
		}
	}
	return found, nil
}

func (m *MemoryStore) UpdateInvite(_ context.Context, i Invite) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.invites[i.ID]; !ok {
		return ErrNotFound
	}
	m.invites[i.ID] = i
	return nil
}

func (m *MemoryStore) DeleteInvite(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.invites[id]; !ok {
		return ErrNotFound
	}
	delete(m.invites, id)
	return nil
}
