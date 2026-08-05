package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/ai"
	"github.com/R7rainz/switchyard/backend/internal/aifeedback"
	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/execution"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
	"github.com/R7rainz/switchyard/backend/internal/workspace"
)

// errNoWorkspace means a route was mounted without the {workspaceID} parameter
// its middleware needs. It is a wiring bug, so it is a 500.
var errNoWorkspace = errors.New("api: route has no workspaceID parameter")

// invalidRequest is something the client can fix: a malformed body, a missing
// field, a role that is not a role. Its detail is safe to return, unlike a
// domain error's — the client wrote the body, so it learns nothing about
// anyone else by being told what is wrong with it.
type invalidRequest struct{ message string }

func (e invalidRequest) Error() string { return "api: " + e.message }

// invalid builds a rejection for writeError, so validation failures pick their
// status in the same place every other error does.
func invalid(format string, args ...any) error {
	return invalidRequest{message: fmt.Sprintf(format, args...)}
}

// writeError turns a domain error into a response. Handlers call this instead
// of choosing status codes themselves, so the mapping stays in one place and
// cannot drift between endpoints.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var bad invalidRequest

	switch {
	case errors.As(err, &bad):
		// 400 before anything else: a body we could not read never reached a
		// rule, so nothing further about it has been decided.
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": bad.message})

	case errors.Is(err, auth.ErrNoIdentity):
		unauthorized(w, "invalid token")

	case errors.Is(err, workflow.ErrInvalid), errors.Is(err, workflow.ErrNotRunnable):
		// The caller wrote the graph, so the reason is theirs to know: "edge
		// 'e1' ends at unknown node 'ghost'" is something they can act on, and
		// it describes only what they just sent.
		//
		// ErrNotRunnable cannot come from a save — a draft that cannot run is
		// still stored. It is here for the execution routes, which reject a
		// run rather than a save, and for the same reason: the graph is the
		// caller's and so is the fix.
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})

	case errors.Is(err, auth.ErrNotOwner),
		errors.Is(err, workspace.ErrNotMember),
		errors.Is(err, workspace.ErrNotFound),
		errors.Is(err, credential.ErrNotFound),
		errors.Is(err, workflow.ErrNotFound),
		errors.Is(err, execution.ErrNotFound):
		// 404, not 403. A 403 would confirm the resource exists, which lets
		// anyone with an account probe for other users' workflow or workspace
		// ids. Someone else's resource and a resource that was never there
		// should be indistinguishable from outside.
		//
		// Not being a member lands here for the same reason. Being told
		// "forbidden" would map out which workspaces exist.
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})

	case errors.Is(err, ai.ErrNoCredential):
		// 400, and the message names the fix: an admin stores an OpenRouter key
		// in the workspace's credentials. Nothing here is secret — the absence
		// of a key is visible to anyone who tries.
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "this workspace has no " + ai.CredentialProvider + " API key; add one in credentials",
		})

	case errors.Is(err, ai.ErrProvider), errors.Is(err, ai.ErrBadGraph):
		// 502: the request was fine and we were fine. Something upstream was
		// not, and the caller's only sensible move is to try again.
		zerolog.Ctx(r.Context()).Warn().Err(err).Msg("model provider failed")
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})

	case errors.Is(err, aifeedback.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})

	case errors.Is(err, workspace.ErrForbidden):
		// A member whose role is too low is a different case: they already
		// know the workspace exists, so 403 tells them nothing new and is the
		// honest answer.
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "insufficient role"})

	case errors.Is(err, workspace.ErrInviteExpired), errors.Is(err, workspace.ErrInviteExhausted):
		// 410: the holder of a real token knows the invite existed, and
		// saying why it stopped working saves a support round trip.
		writeJSON(w, http.StatusGone, map[string]any{"error": err.Error()})

	case errors.Is(err, workspace.ErrSlugTaken):
		// 409 rather than 400: the request was well formed and the caller can
		// retry with a different slug. Nothing here reveals whose workspace
		// holds it, only that the name is gone.
		writeJSON(w, http.StatusConflict, map[string]any{"error": "slug is already taken"})

	case errors.Is(err, execution.ErrNotRunning):
		// 409: the run is real and the caller may see it, it just finished
		// before the cancel arrived. Nothing to retry.
		writeJSON(w, http.StatusConflict, map[string]any{"error": "the run has already finished"})

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
