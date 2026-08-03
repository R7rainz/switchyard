package workspace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/auth"
)

var (
	// ErrNotFound covers a workspace, member, or invite that is not there. It
	// is also what a caller outside a workspace gets instead of a "forbidden",
	// so membership cannot be probed.
	ErrNotFound = errors.New("workspace: not found")

	// ErrNotMember means the caller has no standing in this workspace at all.
	ErrNotMember = errors.New("workspace: caller is not a member")

	// ErrForbidden means the caller is a member, but their role is too low.
	ErrForbidden = errors.New("workspace: role does not permit this")

	// ErrLastOwner guards the invariant that a workspace always has an owner.
	ErrLastOwner = errors.New("workspace: cannot remove the last owner")

	// ErrSlugTaken means the slug belongs to another workspace. Slugs are
	// globally unique because they are meant for URLs, so this is a collision
	// with a stranger's workspace as often as with your own.
	ErrSlugTaken = errors.New("workspace: slug is already taken")
)

// Workspace is the unit everything else belongs to: workflows, executions, and
// credentials are owned by a workspace, never by a user directly.
type Workspace struct {
	ID        string
	Name      string
	Slug      string
	CreatedAt time.Time
}

// Member ties a user to a workspace with a role.
type Member struct {
	WorkspaceID string
	UserID      string
	Role        auth.Role
	CreatedAt   time.Time
}

// Store is the persistence this package needs. The SQL implementation lands
// with the database package; until then MemoryStore stands in.
type Store interface {
	CreateWorkspace(ctx context.Context, w Workspace) error
	GetWorkspace(ctx context.Context, id string) (Workspace, error)

	// ListWorkspacesForUser returns only workspaces the user belongs to. The
	// membership predicate belongs in the query, not in a filter afterwards.
	ListWorkspacesForUser(ctx context.Context, userID string) ([]Workspace, error)

	PutMember(ctx context.Context, m Member) error
	GetMember(ctx context.Context, workspaceID, userID string) (Member, error)
	ListMembers(ctx context.Context, workspaceID string) ([]Member, error)
	DeleteMember(ctx context.Context, workspaceID, userID string) error

	CreateInvite(ctx context.Context, i Invite) error
	GetInviteByTokenHash(ctx context.Context, tokenHash string) (Invite, error)
	ListInvites(ctx context.Context, workspaceID string) ([]Invite, error)
	UpdateInvite(ctx context.Context, i Invite) error
	DeleteInvite(ctx context.Context, id string) error
}

// Service holds the rules about who may do what to a workspace.
type Service struct {
	store Store
	now   func() time.Time

	// newID and newToken are injected so tests are deterministic; production
	// gets crypto/rand through the defaults.
	newID    func() string
	newToken func() (string, error)
}

// NewService returns a Service backed by store.
func NewService(store Store) *Service {
	return &Service{
		store:    store,
		now:      time.Now,
		newID:    randomID,
		newToken: randomToken,
	}
}

// Create makes a workspace and installs its creator as OWNER. The two happen
// together on purpose: a workspace with no owner cannot be administered by
// anyone, so there is no window in which one exists.
func (s *Service) Create(ctx context.Context, creatorID, name, slug string) (Workspace, error) {
	if creatorID == "" {
		return Workspace{}, auth.ErrNoIdentity
	}
	if name == "" || slug == "" {
		return Workspace{}, fmt.Errorf("workspace: name and slug are required")
	}

	created := Workspace{ID: s.newID(), Name: name, Slug: slug, CreatedAt: s.now()}
	if err := s.store.CreateWorkspace(ctx, created); err != nil {
		return Workspace{}, err
	}

	owner := Member{WorkspaceID: created.ID, UserID: creatorID, Role: auth.RoleOwner, CreatedAt: s.now()}
	if err := s.store.PutMember(ctx, owner); err != nil {
		return Workspace{}, err
	}
	return created, nil
}

// RoleOf returns the caller's role in a workspace.
//
// A non-member gets ErrNotMember, which the API layer answers as 404: a 403
// would confirm the workspace exists and let anyone enumerate ids.
func (s *Service) RoleOf(ctx context.Context, workspaceID, userID string) (auth.Role, error) {
	if userID == "" {
		return "", auth.ErrNoIdentity
	}
	member, err := s.store.GetMember(ctx, workspaceID, userID)
	if errors.Is(err, ErrNotFound) {
		return "", ErrNotMember
	}
	if err != nil {
		return "", err
	}
	return member.Role, nil
}

// Authorize reports whether the caller may do p in this workspace, and is the
// single gate every workspace-scoped operation goes through.
func (s *Service) Authorize(ctx context.Context, workspaceID, userID string, p auth.Permission) (auth.Role, error) {
	role, err := s.RoleOf(ctx, workspaceID, userID)
	if err != nil {
		return "", err
	}
	if !role.Can(p) {
		return role, ErrForbidden
	}
	return role, nil
}

// SetRole changes a member's role.
//
// Two rules beyond the permission check, both about privilege escalation: a
// member may not grant a role above their own, and the last owner may not be
// demoted, since that would leave the workspace unadministrable.
func (s *Service) SetRole(ctx context.Context, workspaceID, callerID, targetID string, role auth.Role) error {
	if !role.Valid() {
		return fmt.Errorf("workspace: %q is not a role", role)
	}

	callerRole, err := s.Authorize(ctx, workspaceID, callerID, auth.PermissionMemberManage)
	if err != nil {
		return err
	}
	if !callerRole.AtLeast(role) {
		return ErrForbidden
	}

	target, err := s.store.GetMember(ctx, workspaceID, targetID)
	if err != nil {
		return err
	}
	// Acting on a peer or a senior is escalation by another route.
	if !callerRole.AtLeast(target.Role) || (target.Role == callerRole && callerID != targetID) {
		return ErrForbidden
	}
	if target.Role == auth.RoleOwner && role != auth.RoleOwner {
		if err := s.guardLastOwner(ctx, workspaceID, targetID); err != nil {
			return err
		}
	}

	target.Role = role
	return s.store.PutMember(ctx, target)
}

// Remove takes a member out of a workspace. Members may always remove
// themselves; removing anyone else needs member:manage and a senior role.
func (s *Service) Remove(ctx context.Context, workspaceID, callerID, targetID string) error {
	if callerID != targetID {
		callerRole, err := s.Authorize(ctx, workspaceID, callerID, auth.PermissionMemberManage)
		if err != nil {
			return err
		}
		target, err := s.store.GetMember(ctx, workspaceID, targetID)
		if err != nil {
			return err
		}
		if !callerRole.AtLeast(target.Role) || target.Role == callerRole {
			return ErrForbidden
		}
	} else if _, err := s.RoleOf(ctx, workspaceID, callerID); err != nil {
		return err
	}

	if err := s.guardLastOwner(ctx, workspaceID, targetID); err != nil {
		return err
	}
	return s.store.DeleteMember(ctx, workspaceID, targetID)
}

// List returns the workspaces this user belongs to.
//
// No permission check: membership is the filter. The query returns only
// workspaces the caller is in, so there is nothing here they could not see.
func (s *Service) List(ctx context.Context, userID string) ([]Workspace, error) {
	if userID == "" {
		return nil, auth.ErrNoIdentity
	}
	return s.store.ListWorkspacesForUser(ctx, userID)
}

// Members lists a workspace's members for a caller allowed to see them.
func (s *Service) Members(ctx context.Context, workspaceID, callerID string) ([]Member, error) {
	if _, err := s.Authorize(ctx, workspaceID, callerID, auth.PermissionMemberRead); err != nil {
		return nil, err
	}
	return s.store.ListMembers(ctx, workspaceID)
}

// guardLastOwner refuses to strip ownership from the only owner left.
func (s *Service) guardLastOwner(ctx context.Context, workspaceID, targetID string) error {
	target, err := s.store.GetMember(ctx, workspaceID, targetID)
	if err != nil {
		return err
	}
	if target.Role != auth.RoleOwner {
		return nil
	}

	members, err := s.store.ListMembers(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.Role == auth.RoleOwner && m.UserID != targetID {
			return nil
		}
	}
	return ErrLastOwner
}
