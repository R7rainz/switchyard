package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/auth"
)

func TestInviteReturnsATokenThatIsNotStored(t *testing.T) {
	svc, workspaceID := newTestService(t)

	invite, token, err := svc.Invite(context.Background(), workspaceID, "owner-1", InviteRequest{
		Email: "new@switchyard.test",
		Role:  auth.RoleMember,
	})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	if token == "" {
		t.Fatal("Invite returned no token")
	}
	// The token is a bearer credential. Anyone reading the database must not
	// be able to join with what they find there.
	if strings.Contains(invite.TokenHash, token) || invite.TokenHash == token {
		t.Error("the raw token was stored")
	}
	if invite.TokenHash != HashToken(token) {
		t.Error("stored hash does not match the token")
	}
}

func TestInviteCannotGrantMoreThanTheInviterHas(t *testing.T) {
	svc, workspaceID := newTestService(t)
	join(t, svc, workspaceID, "admin-1", auth.RoleAdmin)
	ctx := context.Background()

	// An admin handing out OWNER would be handing away the workspace.
	if _, _, err := svc.Invite(ctx, workspaceID, "admin-1", InviteRequest{Role: auth.RoleOwner}); !errors.Is(err, ErrForbidden) {
		t.Errorf("admin invited an owner: err = %v", err)
	}
	if _, _, err := svc.Invite(ctx, workspaceID, "admin-1", InviteRequest{Role: auth.RoleAdmin}); err != nil {
		t.Errorf("admin could not invite a peer: %v", err)
	}

	// A member cannot invite at all.
	join(t, svc, workspaceID, "member-1", auth.RoleMember)
	if _, _, err := svc.Invite(ctx, workspaceID, "member-1", InviteRequest{Role: auth.RoleViewer}); !errors.Is(err, ErrForbidden) {
		t.Errorf("member sent an invite: err = %v", err)
	}

	// Nor can a stranger.
	if _, _, err := svc.Invite(ctx, workspaceID, "stranger", InviteRequest{Role: auth.RoleViewer}); !errors.Is(err, ErrNotMember) {
		t.Errorf("stranger sent an invite: err = %v", err)
	}
}

func TestAcceptMakesAMember(t *testing.T) {
	svc, workspaceID := newTestService(t)
	ctx := context.Background()

	_, token, err := svc.Invite(ctx, workspaceID, "owner-1", InviteRequest{Email: "new@switchyard.test", Role: auth.RoleMember})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	joined, err := svc.Accept(ctx, token, "new-user")
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if joined.ID != workspaceID {
		t.Errorf("joined %s, want %s", joined.ID, workspaceID)
	}

	role, err := svc.RoleOf(ctx, workspaceID, "new-user")
	if err != nil {
		t.Fatalf("RoleOf: %v", err)
	}
	if role != auth.RoleMember {
		t.Errorf("role = %s, want MEMBER", role)
	}
}

func TestAcceptRejectsAnUnknownToken(t *testing.T) {
	svc, _ := newTestService(t)

	if _, err := svc.Accept(context.Background(), "not-a-real-token", "someone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestAddressedInviteIsSingleUse(t *testing.T) {
	svc, workspaceID := newTestService(t)
	ctx := context.Background()

	_, token, err := svc.Invite(ctx, workspaceID, "owner-1", InviteRequest{Email: "one@switchyard.test", Role: auth.RoleMember})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	if _, err := svc.Accept(ctx, token, "first-user"); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	// The invite was consumed, so a forwarded email cannot admit a second
	// person.
	if _, err := svc.Accept(ctx, token, "second-user"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a consumed invite admitted a second user: err = %v", err)
	}
}

func TestLinkInviteAdmitsUpToMaxUses(t *testing.T) {
	svc, workspaceID := newTestService(t)
	ctx := context.Background()

	_, token, err := svc.Invite(ctx, workspaceID, "owner-1", InviteRequest{Role: auth.RoleViewer, MaxUses: 2})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	for _, user := range []string{"user-1", "user-2"} {
		if _, err := svc.Accept(ctx, token, user); err != nil {
			t.Fatalf("Accept for %s: %v", user, err)
		}
	}
	if _, err := svc.Accept(ctx, token, "user-3"); !errors.Is(err, ErrInviteExhausted) {
		t.Errorf("err = %v, want ErrInviteExhausted", err)
	}
}

func TestExpiredInviteIsRefused(t *testing.T) {
	svc, workspaceID := newTestService(t)
	ctx := context.Background()

	now := time.Now()
	svc.now = func() time.Time { return now }

	_, token, err := svc.Invite(ctx, workspaceID, "owner-1", InviteRequest{Role: auth.RoleViewer, TTL: time.Hour})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	svc.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := svc.Accept(ctx, token, "late-user"); !errors.Is(err, ErrInviteExpired) {
		t.Errorf("err = %v, want ErrInviteExpired", err)
	}
}

func TestInviteExpiryDefaults(t *testing.T) {
	svc, workspaceID := newTestService(t)
	ctx := context.Background()
	now := time.Now()
	svc.now = func() time.Time { return now }

	withDefault, _, err := svc.Invite(ctx, workspaceID, "owner-1", InviteRequest{Role: auth.RoleViewer})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if got, want := withDefault.ExpiresAt, now.Add(defaultInviteTTL); !got.Equal(want) {
		t.Errorf("default expiry = %v, want %v", got, want)
	}

	// A link that never expires has to be asked for.
	eternal, _, err := svc.Invite(ctx, workspaceID, "owner-1", InviteRequest{Role: auth.RoleViewer, TTL: -1})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if !eternal.ExpiresAt.IsZero() {
		t.Errorf("expiry = %v, want none", eternal.ExpiresAt)
	}
}

func TestAcceptingTwiceKeepsTheRoleYouHave(t *testing.T) {
	svc, workspaceID := newTestService(t)
	ctx := context.Background()
	join(t, svc, workspaceID, "admin-1", auth.RoleAdmin)

	// A low-privilege link must not be usable to demote an existing admin.
	_, token, err := svc.Invite(ctx, workspaceID, "owner-1", InviteRequest{Role: auth.RoleViewer, MaxUses: 5})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	if _, err := svc.Accept(ctx, token, "admin-1"); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	role, err := svc.RoleOf(ctx, workspaceID, "admin-1")
	if err != nil {
		t.Fatalf("RoleOf: %v", err)
	}
	if role != auth.RoleAdmin {
		t.Errorf("role = %s, want ADMIN — an invite demoted an existing member", role)
	}
}

func TestRevoke(t *testing.T) {
	svc, workspaceID := newTestService(t)
	ctx := context.Background()

	invite, token, err := svc.Invite(ctx, workspaceID, "owner-1", InviteRequest{Role: auth.RoleViewer, MaxUses: 5})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	join(t, svc, workspaceID, "member-1", auth.RoleMember)
	if err := svc.Revoke(ctx, workspaceID, "member-1", invite.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("member revoked an invite: err = %v", err)
	}

	if err := svc.Revoke(ctx, workspaceID, "owner-1", invite.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := svc.Accept(ctx, token, "someone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a revoked invite still worked: err = %v", err)
	}
}

func TestInvitesListingCarriesNoUsableToken(t *testing.T) {
	svc, workspaceID := newTestService(t)
	ctx := context.Background()

	_, token, err := svc.Invite(ctx, workspaceID, "owner-1", InviteRequest{Role: auth.RoleViewer})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	listed, err := svc.Invites(ctx, workspaceID, "owner-1")
	if err != nil {
		t.Fatalf("Invites: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d invites, want 1", len(listed))
	}
	if listed[0].TokenHash == token {
		t.Error("the listing exposes the raw token")
	}

	// A viewer has no business seeing pending invitations.
	join(t, svc, workspaceID, "viewer-1", auth.RoleViewer)
	if _, err := svc.Invites(ctx, workspaceID, "viewer-1"); !errors.Is(err, ErrForbidden) {
		t.Errorf("viewer listed invites: err = %v", err)
	}
}
