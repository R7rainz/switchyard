package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/auth"
)

var (
	// ErrInviteExpired and ErrInviteExhausted are separate from ErrNotFound
	// because the holder of a real token already knows the invite existed;
	// telling them why it stopped working leaks nothing and saves a support
	// round trip.
	ErrInviteExpired   = errors.New("workspace: invite has expired")
	ErrInviteExhausted = errors.New("workspace: invite has no uses left")
)

// defaultInviteTTL matches the usual expectation for an emailed invite. A link
// with no expiry has to be asked for explicitly.
const defaultInviteTTL = 7 * 24 * time.Hour

// Invite grants membership at a fixed role to whoever presents its token.
//
// Two shapes share this record. An invite with an Email is aimed at one
// person and is consumed when used. An invite without one is a shareable
// link, which may admit several people until MaxUses runs out.
type Invite struct {
	ID          string
	WorkspaceID string

	// TokenHash is the SHA-256 of the token, hex encoded. The token itself is
	// a bearer credential and is never stored: anyone reading the database
	// would otherwise be able to join every workspace with an open invite. It
	// is returned exactly once, when the invite is created.
	TokenHash string

	Email     string
	Role      auth.Role
	ExpiresAt time.Time // zero means it never expires
	MaxUses   int       // zero means unlimited
	UseCount  int
	InvitedBy string
	CreatedAt time.Time
}

// IsLink reports whether this is a shareable invite rather than one addressed
// to a person.
func (i Invite) IsLink() bool { return i.Email == "" }

// Expired reports whether the invite is past its expiry at the given moment.
func (i Invite) Expired(now time.Time) bool {
	return !i.ExpiresAt.IsZero() && now.After(i.ExpiresAt)
}

// Exhausted reports whether a capped invite has been used up.
func (i Invite) Exhausted() bool {
	return i.MaxUses > 0 && i.UseCount >= i.MaxUses
}

// InviteRequest describes an invite to create.
type InviteRequest struct {
	// Email addresses the invite to one person. Empty makes a shareable link.
	Email string

	// Role the invitee receives. It may not exceed the inviter's own role.
	Role auth.Role

	// TTL overrides the seven-day default. Negative means no expiry.
	TTL time.Duration

	// MaxUses caps a shareable link. Zero is unlimited; it is ignored for an
	// emailed invite, which is always single use.
	MaxUses int
}

// Invite creates an invitation and returns it with its one-time token. The
// token is not recoverable afterwards — revoke and re-issue instead.
func (s *Service) Invite(ctx context.Context, workspaceID, callerID string, req InviteRequest) (Invite, string, error) {
	callerRole, err := s.Authorize(ctx, workspaceID, callerID, auth.PermissionMemberManage)
	if err != nil {
		return Invite{}, "", err
	}
	if !req.Role.Valid() {
		return Invite{}, "", errors.New("workspace: invite role is not a role")
	}
	// Nobody hands out more than they hold, or an admin would mint an owner.
	if !callerRole.AtLeast(req.Role) {
		return Invite{}, "", ErrForbidden
	}

	token, err := s.newToken()
	if err != nil {
		return Invite{}, "", err
	}

	now := s.now()
	invite := Invite{
		ID:          s.newID(),
		WorkspaceID: workspaceID,
		TokenHash:   HashToken(token),
		Email:       strings.ToLower(strings.TrimSpace(req.Email)),
		Role:        req.Role,
		UseCount:    0,
		InvitedBy:   callerID,
		CreatedAt:   now,
	}

	switch {
	case req.TTL < 0:
		// Explicitly eternal.
	case req.TTL == 0:
		invite.ExpiresAt = now.Add(defaultInviteTTL)
	default:
		invite.ExpiresAt = now.Add(req.TTL)
	}

	// An addressed invite is for one person, so a use cap would be meaningless.
	if !invite.IsLink() {
		invite.MaxUses = 1
	} else {
		invite.MaxUses = req.MaxUses
	}

	if err := s.store.CreateInvite(ctx, invite); err != nil {
		return Invite{}, "", err
	}
	return invite, token, nil
}

// Accept redeems a token and makes the caller a member.
//
// Accepting twice is not an error: the caller ends up a member either way, and
// failing the second attempt would only confuse someone who double-clicked.
// An existing member keeps the role they already have, so a low-privilege
// invite cannot be used to demote an admin.
func (s *Service) Accept(ctx context.Context, token, userID string) (Workspace, error) {
	if userID == "" {
		return Workspace{}, auth.ErrNoIdentity
	}

	invite, err := s.store.GetInviteByTokenHash(ctx, HashToken(token))
	if err != nil {
		return Workspace{}, err
	}

	now := s.now()
	if invite.Expired(now) {
		return Workspace{}, ErrInviteExpired
	}
	if invite.Exhausted() {
		return Workspace{}, ErrInviteExhausted
	}

	workspace, err := s.store.GetWorkspace(ctx, invite.WorkspaceID)
	if err != nil {
		return Workspace{}, err
	}

	existing, err := s.store.GetMember(ctx, invite.WorkspaceID, userID)
	switch {
	case err == nil:
		// Already in. Consume an addressed invite so the link stops working,
		// and leave their role alone.
		if !invite.IsLink() {
			if err := s.store.DeleteInvite(ctx, invite.ID); err != nil {
				return Workspace{}, err
			}
		}
		_ = existing
		return workspace, nil
	case !errors.Is(err, ErrNotFound):
		return Workspace{}, err
	}

	member := Member{WorkspaceID: invite.WorkspaceID, UserID: userID, Role: invite.Role, CreatedAt: now}
	if err := s.store.PutMember(ctx, member); err != nil {
		return Workspace{}, err
	}

	if invite.IsLink() {
		invite.UseCount++
		if err := s.store.UpdateInvite(ctx, invite); err != nil {
			return Workspace{}, err
		}
	} else if err := s.store.DeleteInvite(ctx, invite.ID); err != nil {
		return Workspace{}, err
	}

	return workspace, nil
}

// Invites lists a workspace's invitations. The records carry token hashes, so
// nothing here can be used to join.
func (s *Service) Invites(ctx context.Context, workspaceID, callerID string) ([]Invite, error) {
	if _, err := s.Authorize(ctx, workspaceID, callerID, auth.PermissionMemberManage); err != nil {
		return nil, err
	}
	return s.store.ListInvites(ctx, workspaceID)
}

// Revoke deletes an invitation.
func (s *Service) Revoke(ctx context.Context, workspaceID, callerID, inviteID string) error {
	if _, err := s.Authorize(ctx, workspaceID, callerID, auth.PermissionMemberManage); err != nil {
		return err
	}
	return s.store.DeleteInvite(ctx, inviteID)
}

// HashToken returns the stored form of an invite token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// randomToken returns 32 bytes of entropy, hex encoded — far past guessing,
// and safe to put in a URL.
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomID() string {
	buf := make([]byte, 16)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}
