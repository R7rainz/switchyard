package api

import (
	"errors"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/workspace"
)

// errNoWorkspace means a route was mounted without the {workspaceID} parameter
// its middleware needs. It is a wiring bug, so it is a 500.
var errNoWorkspace = errors.New("api: route has no workspaceID parameter")

// writeError turns a domain error into a response. Handlers call this instead
// of choosing status codes themselves, so the mapping stays in one place and
// cannot drift between endpoints.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrNoIdentity):
		unauthorized(w, "invalid token")

	case errors.Is(err, auth.ErrNotOwner),
		errors.Is(err, workspace.ErrNotMember),
		errors.Is(err, workspace.ErrNotFound):
		// 404, not 403. A 403 would confirm the resource exists, which lets
		// anyone with an account probe for other users' workflow or workspace
		// ids. Someone else's resource and a resource that was never there
		// should be indistinguishable from outside.
		//
		// Not being a member lands here for the same reason. Being told
		// "forbidden" would map out which workspaces exist.
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})

	case errors.Is(err, workspace.ErrForbidden):
		// A member whose role is too low is a different case: they already
		// know the workspace exists, so 403 tells them nothing new and is the
		// honest answer.
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "insufficient role"})

	case errors.Is(err, workspace.ErrInviteExpired), errors.Is(err, workspace.ErrInviteExhausted):
		// 410: the holder of a real token knows the invite existed, and
		// saying why it stopped working saves a support round trip.
		writeJSON(w, http.StatusGone, map[string]any{"error": err.Error()})

	case errors.Is(err, workspace.ErrLastOwner):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "a workspace must keep an owner"})

	default:
		// The cause goes to the log; the client gets nothing it could use to
		// map our internals.
		zerolog.Ctx(r.Context()).Error().
			Err(err).
			Str("path", r.URL.Path).
			Msg("request failed")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error"})
	}
}
