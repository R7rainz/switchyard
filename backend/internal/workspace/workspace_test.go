package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/auth"
)

// newTestService returns a service with deterministic ids, plus the workspace
// its first user owns.
func newTestService(t *testing.T) (*Service, string) {
	t.Helper()

	counter := 0
	svc := NewService(NewMemoryStore())
	svc.newID = func() string {
		counter++
		return "id-" + string(rune('a'+counter-1))
	}

	created, err := svc.Create(context.Background(), "owner-1", "Switchyard", "switchyard")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return svc, created.ID
}

// join puts a user in the workspace at a role, bypassing invites.
func join(t *testing.T, svc *Service, workspaceID, userID string, role auth.Role) {
	t.Helper()
	member := Member{WorkspaceID: workspaceID, UserID: userID, Role: role, CreatedAt: svc.now()}
	if err := svc.store.PutMember(context.Background(), member); err != nil {
		t.Fatalf("PutMember: %v", err)
	}
}

func TestCreateInstallsCreatorAsOwner(t *testing.T) {
	svc, workspaceID := newTestService(t)

	role, err := svc.RoleOf(context.Background(), workspaceID, "owner-1")
	if err != nil {
		t.Fatalf("RoleOf: %v", err)
	}
	if role != auth.RoleOwner {
		t.Errorf("creator role = %s, want OWNER", role)
	}
}

func TestNonMemberIsNotToldTheWorkspaceExists(t *testing.T) {
	svc, workspaceID := newTestService(t)

	// ErrNotMember, which the API answers as 404 — a 403 would confirm the id
	// is real and let anyone enumerate workspaces.
	if _, err := svc.RoleOf(context.Background(), workspaceID, "stranger"); !errors.Is(err, ErrNotMember) {
		t.Errorf("err = %v, want ErrNotMember", err)
	}
	if _, err := svc.Authorize(context.Background(), workspaceID, "stranger", auth.PermissionWorkflowRead); !errors.Is(err, ErrNotMember) {
		t.Errorf("err = %v, want ErrNotMember", err)
	}
}

func TestAuthorizeChecksTheRole(t *testing.T) {
	svc, workspaceID := newTestService(t)
	join(t, svc, workspaceID, "viewer-1", auth.RoleViewer)

	if _, err := svc.Authorize(context.Background(), workspaceID, "viewer-1", auth.PermissionWorkflowRead); err != nil {
		t.Errorf("viewer denied a read: %v", err)
	}
	if _, err := svc.Authorize(context.Background(), workspaceID, "viewer-1", auth.PermissionWorkflowWrite); !errors.Is(err, ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestSetRoleRefusesEscalation(t *testing.T) {
	svc, workspaceID := newTestService(t)
	join(t, svc, workspaceID, "admin-1", auth.RoleAdmin)
	join(t, svc, workspaceID, "member-1", auth.RoleMember)
	join(t, svc, workspaceID, "admin-2", auth.RoleAdmin)

	ctx := context.Background()

	// An admin may promote a member up to their own level.
	if err := svc.SetRole(ctx, workspaceID, "admin-1", "member-1", auth.RoleAdmin); err != nil {
		t.Errorf("admin could not promote a member to admin: %v", err)
	}

	// But not past it, or an admin mints an owner and takes the workspace.
	join(t, svc, workspaceID, "member-2", auth.RoleMember)
	if err := svc.SetRole(ctx, workspaceID, "admin-1", "member-2", auth.RoleOwner); !errors.Is(err, ErrForbidden) {
		t.Errorf("admin granted OWNER: err = %v", err)
	}

	// And not against a peer, which is escalation by another route.
	if err := svc.SetRole(ctx, workspaceID, "admin-1", "admin-2", auth.RoleViewer); !errors.Is(err, ErrForbidden) {
		t.Errorf("admin demoted a peer: err = %v", err)
	}

	// A member cannot touch roles at all.
	if err := svc.SetRole(ctx, workspaceID, "member-2", "member-2", auth.RoleAdmin); !errors.Is(err, ErrForbidden) {
		t.Errorf("member promoted themselves: err = %v", err)
	}
}

func TestSetRoleRejectsUnknownRole(t *testing.T) {
	svc, workspaceID := newTestService(t)
	join(t, svc, workspaceID, "member-1", auth.RoleMember)

	if err := svc.SetRole(context.Background(), workspaceID, "owner-1", "member-1", "SUPERUSER"); err == nil {
		t.Error("SetRole accepted a role that does not exist")
	}
}

func TestLastOwnerCannotBeDemotedOrRemoved(t *testing.T) {
	svc, workspaceID := newTestService(t)
	ctx := context.Background()

	if err := svc.SetRole(ctx, workspaceID, "owner-1", "owner-1", auth.RoleAdmin); !errors.Is(err, ErrLastOwner) {
		t.Errorf("demoting the only owner: err = %v, want ErrLastOwner", err)
	}
	if err := svc.Remove(ctx, workspaceID, "owner-1", "owner-1"); !errors.Is(err, ErrLastOwner) {
		t.Errorf("removing the only owner: err = %v, want ErrLastOwner", err)
	}

	// With a second owner in place, the first may step down.
	join(t, svc, workspaceID, "owner-2", auth.RoleOwner)
	if err := svc.SetRole(ctx, workspaceID, "owner-1", "owner-1", auth.RoleAdmin); err != nil {
		t.Errorf("owner could not step down beside a second owner: %v", err)
	}
}

func TestRemove(t *testing.T) {
	svc, workspaceID := newTestService(t)
	join(t, svc, workspaceID, "admin-1", auth.RoleAdmin)
	join(t, svc, workspaceID, "member-1", auth.RoleMember)
	join(t, svc, workspaceID, "admin-2", auth.RoleAdmin)
	ctx := context.Background()

	// Anyone may leave under their own steam.
	join(t, svc, workspaceID, "viewer-1", auth.RoleViewer)
	if err := svc.Remove(ctx, workspaceID, "viewer-1", "viewer-1"); err != nil {
		t.Errorf("member could not remove themselves: %v", err)
	}

	// A senior may remove a junior.
	if err := svc.Remove(ctx, workspaceID, "admin-1", "member-1"); err != nil {
		t.Errorf("admin could not remove a member: %v", err)
	}

	// A peer may not.
	if err := svc.Remove(ctx, workspaceID, "admin-1", "admin-2"); !errors.Is(err, ErrForbidden) {
		t.Errorf("admin removed a peer: err = %v", err)
	}

	// A junior certainly may not remove a senior.
	join(t, svc, workspaceID, "member-2", auth.RoleMember)
	if err := svc.Remove(ctx, workspaceID, "member-2", "admin-1"); !errors.Is(err, ErrForbidden) {
		t.Errorf("member removed an admin: err = %v", err)
	}

	// And a stranger cannot remove anyone.
	if err := svc.Remove(ctx, workspaceID, "stranger", "admin-1"); !errors.Is(err, ErrNotMember) {
		t.Errorf("stranger removed a member: err = %v", err)
	}
}

func TestMembersRequiresMembership(t *testing.T) {
	svc, workspaceID := newTestService(t)
	join(t, svc, workspaceID, "viewer-1", auth.RoleViewer)

	if _, err := svc.Members(context.Background(), workspaceID, "viewer-1"); err != nil {
		t.Errorf("viewer could not list members: %v", err)
	}
	if _, err := svc.Members(context.Background(), workspaceID, "stranger"); !errors.Is(err, ErrNotMember) {
		t.Errorf("stranger listed members: err = %v", err)
	}
}

func TestListWorkspacesForUserOnlyReturnsTheirs(t *testing.T) {
	svc, workspaceID := newTestService(t)
	ctx := context.Background()

	other, err := svc.Create(ctx, "owner-2", "Other", "other")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	mine, err := svc.store.ListWorkspacesForUser(ctx, "owner-1")
	if err != nil {
		t.Fatalf("ListWorkspacesForUser: %v", err)
	}
	if len(mine) != 1 || mine[0].ID != workspaceID {
		t.Fatalf("owner-1 sees %v, want only %s", mine, workspaceID)
	}
	if mine[0].ID == other.ID {
		t.Error("owner-1 can see another user's workspace")
	}
}
