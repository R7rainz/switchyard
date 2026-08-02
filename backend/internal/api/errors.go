package api

import (
	"errors"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/auth"
)

// writeError turns a domain error into a response. Handlers call this instead
// of choosing status codes themselves, so the mapping stays in one place and
// cannot drift between endpoints.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrNoIdentity):
		unauthorized(w, "invalid token")

	case errors.Is(err, auth.ErrNotOwner):
		// 404, not 403. A 403 would confirm the resource exists, which lets
		// anyone with an account probe for other users' workflow ids. Someone
		// else's resource and a resource that was never there should be
		// indistinguishable from outside.
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})

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
