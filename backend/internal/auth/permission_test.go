package auth

import (
	"context"
	"errors"
	"testing"
)

func authenticated(subject string) context.Context {
	return NewContext(context.Background(), &Claims{Subject: subject})
}

func TestUserID(t *testing.T) {
	id, err := UserID(authenticated("user-123"))
	if err != nil {
		t.Fatalf("UserID: %v", err)
	}
	if id != "user-123" {
		t.Errorf("UserID = %q, want user-123", id)
	}
}

func TestUserIDWithoutIdentity(t *testing.T) {
	tests := map[string]context.Context{
		"bare context":  context.Background(),
		"empty subject": authenticated(""),
	}

	for name, ctx := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := UserID(ctx); !errors.Is(err, ErrNoIdentity) {
				t.Errorf("err = %v, want ErrNoIdentity", err)
			}
		})
	}
}

func TestRequireOwner(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		ownerID string
		want    error
	}{
		{
			name:    "owner",
			ctx:     authenticated("user-123"),
			ownerID: "user-123",
			want:    nil,
		},
		{
			name:    "another user's resource",
			ctx:     authenticated("user-123"),
			ownerID: "user-456",
			want:    ErrNotOwner,
		},
		{
			// An unset owner must never match. Treating it as public would
			// turn a bug elsewhere into a leak.
			name:    "resource with no owner",
			ctx:     authenticated("user-123"),
			ownerID: "",
			want:    ErrNotOwner,
		},
		{
			name:    "unauthenticated caller",
			ctx:     context.Background(),
			ownerID: "user-123",
			want:    ErrNoIdentity,
		},
		{
			// Both empty must not be read as "the empty user owns it".
			name:    "no identity and no owner",
			ctx:     context.Background(),
			ownerID: "",
			want:    ErrNoIdentity,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireOwner(tc.ctx, tc.ownerID)
			if !errors.Is(err, tc.want) {
				t.Errorf("RequireOwner = %v, want %v", err, tc.want)
			}
		})
	}
}
