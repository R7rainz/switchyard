package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/R7rainz/switchyard/backend/internal/auth"
)

// Authorizer is the part of the workspace service this layer needs: given a
// workspace, a caller, and a permission, say yes or why not.
type Authorizer interface {
	Authorize(ctx context.Context, workspaceID, userID string, p auth.Permission) (auth.Role, error)
}

// workspaceContextKey carries the caller's role for the handlers below.
type workspaceContextKey struct{}

// WorkspaceRole returns the role the caller holds in the workspace the URL
// names. It is only present below RequirePermission.
func WorkspaceRole(ctx context.Context) (auth.Role, bool) {
	role, ok := ctx.Value(workspaceContextKey{}).(auth.Role)
	return role, ok
}

// RequirePermission gates a route on the caller holding p in the workspace
// named by the {workspaceID} URL parameter.
//
// The check runs per request rather than off the token: roles change, and a
// token minted before a demotion must not keep the access it was minted with.
func RequirePermission(authorizer Authorizer, p auth.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := auth.UserID(r.Context())
			if err != nil {
				writeError(w, r, err)
				return
			}

			workspaceID := chi.URLParam(r, "workspaceID")
			if workspaceID == "" {
				// A route mounted without the parameter would otherwise check
				// permission against the empty workspace and pass nobody, or
				// worse, be read as global.
				writeError(w, r, errNoWorkspace)
				return
			}

			role, err := authorizer.Authorize(r.Context(), workspaceID, userID, p)
			if err != nil {
				writeError(w, r, err)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), workspaceContextKey{}, role)))
		})
	}
}
