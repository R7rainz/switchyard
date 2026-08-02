package auth

import (
	"context"
	"errors"
)

// The v1 permission model is ownership, and nothing else. A workflow,
// execution, or credential belongs to exactly one user, and that user is the
// only one who may touch it. There are no roles, no teams, and no sharing, so
// there is no permission to grant beyond "is this yours".
//
// Keeping the model this small is deliberate. The enforcement that matters is
// a predicate on the query — WHERE owner_id = the caller — so that forgetting
// to check cannot silently return someone else's row. RequireOwner is the
// backstop for the paths where a resource is already in hand.
var (
	// ErrNoIdentity means the request never carried a verified token. Reaching
	// this from a handler behind RequireAuth is a wiring mistake, not a user
	// error.
	ErrNoIdentity = errors.New("auth: no identity on context")

	// ErrNotOwner means an authenticated caller asked for someone else's
	// resource.
	ErrNotOwner = errors.New("auth: caller does not own this resource")
)

// UserID returns the caller's user id, which is the value every owner-scoped
// query must filter by.
func UserID(ctx context.Context) (string, error) {
	claims, ok := FromContext(ctx)
	if !ok || claims.Subject == "" {
		return "", ErrNoIdentity
	}
	return claims.Subject, nil
}

// RequireOwner reports whether the caller owns a resource already loaded.
//
// An empty ownerID never matches. A row whose owner was never set is a bug
// somewhere else, and treating it as public would turn that bug into a leak.
func RequireOwner(ctx context.Context, ownerID string) error {
	caller, err := UserID(ctx)
	if err != nil {
		return err
	}
	if ownerID == "" || caller != ownerID {
		return ErrNotOwner
	}
	return nil
}
