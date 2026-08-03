package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/execution"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
	"github.com/R7rainz/switchyard/backend/internal/workspace"
)

// TokenVerifier is the part of the auth package this layer needs. It is
// declared here, where it is consumed, so handlers can be tested without a
// live JWKS endpoint.
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (*auth.Claims, error)
}

// NewRouter builds the HTTP surface. Routing, encoding, and auth enforcement
// live here; everything else belongs to the domain packages.
//
// appURL is the frontend's base URL, which is where an invite link has to
// point: the invitee needs a page, not this API.
func NewRouter(
	verifier TokenVerifier,
	logger zerolog.Logger,
	workspaces *workspace.Service,
	credentials *credential.Service,
	workflows *workflow.Service,
	executions *execution.Service,
	appURL string,
) http.Handler {
	router := chi.NewRouter()

	// RequestID first, so every line the other middleware logs can be tied
	// back to one request.
	router.Use(middleware.RequestID)
	router.Use(RequestLogger(logger))
	// Before RequireAuth, because a preflight carries no Authorization header
	// and a 401 would stop the real request from ever being sent.
	router.Use(CORS(appURL))
	// A panic in one handler should fail one request, not the process.
	router.Use(middleware.Recoverer)

	// Unauthenticated: this is what a load balancer polls.
	router.Get("/healthz", handleHealthz)

	ws := &workspaceAPI{workspaces: workspaces, appURL: appURL}
	creds := &credentialAPI{credentials: credentials}
	flows := &workflowAPI{workflows: workflows}
	runs := &executionAPI{executions: executions}

	router.Route("/api", func(r chi.Router) {
		r.Use(RequireAuth(verifier, logger))
		r.Get("/me", handleMe)

		r.Get("/workspaces", ws.listWorkspaces)
		r.Post("/workspaces", ws.createWorkspace)

		r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
			read := RequirePermission(workspaces, auth.PermissionMemberRead)
			manage := RequirePermission(workspaces, auth.PermissionMemberManage)

			r.With(read).Get("/members", ws.listMembers)
			r.With(manage).Patch("/members/{userID}", ws.setRole)
			// Leaving is gated on membership alone, not member:manage: the
			// service already requires manage to remove somebody else, and a
			// viewer who could never leave would be stuck in a workspace they
			// were invited to by mistake.
			r.With(read).Delete("/members/{userID}", ws.removeMember)

			r.With(manage).Post("/invites", ws.createInvite)
			r.With(manage).Get("/invites", ws.listInvites)
			r.With(manage).Delete("/invites/{inviteID}", ws.revokeInvite)

			// Credentials sit at ADMIN even to list. A member runs workflows
			// that use the keys; which keys exist is a different question, and
			// the listing alone tells you which providers a workspace is
			// wired to.
			keys := RequirePermission(workspaces, auth.PermissionCredentialManage)
			r.With(keys).Get("/credentials", creds.listCredentials)
			r.With(keys).Put("/credentials/{provider}/{name}", creds.putCredential)
			r.With(keys).Delete("/credentials/{provider}/{name}", creds.deleteCredential)

			// Delete is its own permission rather than folded into write: a
			// member may edit a workflow they did not draw, and deleting one is
			// the change that cannot be undone by editing it back.
			readFlows := RequirePermission(workspaces, auth.PermissionWorkflowRead)
			writeFlows := RequirePermission(workspaces, auth.PermissionWorkflowWrite)
			dropFlows := RequirePermission(workspaces, auth.PermissionWorkflowDelete)

			r.With(readFlows).Get("/workflows", flows.listWorkflows)
			r.With(writeFlows).Post("/workflows", flows.createWorkflow)
			r.With(readFlows).Get("/workflows/{workflowID}", flows.getWorkflow)
			r.With(writeFlows).Patch("/workflows/{workflowID}", flows.updateWorkflow)
			r.With(dropFlows).Delete("/workflows/{workflowID}", flows.deleteWorkflow)

			// Running is a separate permission from editing: a VIEWER may watch
			// what happened, and it takes a MEMBER to make something happen.
			// Cancelling sits with running rather than reading, since stopping
			// a run is an action on it.
			readRuns := RequirePermission(workspaces, auth.PermissionExecutionRead)
			runRuns := RequirePermission(workspaces, auth.PermissionExecutionRun)

			r.With(runRuns).Post("/workflows/{workflowID}/executions", runs.startExecution)
			r.With(readRuns).Get("/executions", runs.listExecutions)
			r.With(readRuns).Get("/executions/{executionID}", runs.getExecution)
			r.With(runRuns).Post("/executions/{executionID}/cancel", runs.cancelExecution)
		})

		// Accepting cannot be workspace-scoped: the caller holds no membership
		// yet, which is the entire point of an invite. It still needs a token,
		// because the invite grants membership to a user, not to a browser.
		r.Post("/invites/{token}/accept", ws.acceptInvite)
	})

	return router
}

// RequestLogger records one line per request, after it completes, so the status
// and duration are known.
func RequestLogger(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Carry the logger on the context so handlers below can log
			// without every signature growing a logger parameter.
			r = r.WithContext(logger.WithContext(r.Context()))

			next.ServeHTTP(recorder, r)

			// Health checks arrive every few seconds and say nothing, so they
			// log at debug and stay out of the way.
			level := zerolog.InfoLevel
			switch {
			case r.URL.Path == "/healthz":
				level = zerolog.DebugLevel
			case recorder.Status() >= http.StatusInternalServerError:
				level = zerolog.ErrorLevel
			}

			logger.WithLevel(level).
				Str("method", r.Method).
				Str("path", logPath(r.URL.Path)).
				Int("status", recorder.Status()).
				Int("bytes", recorder.BytesWritten()).
				Dur("duration", time.Since(started)).
				Str("request_id", middleware.GetReqID(r.Context())).
				Msg("request")
		})
	}
}

// RequireAuth returns middleware that rejects any request without a valid
// Better Auth bearer token, and puts the verified claims on the request
// context for the handlers below it.
func RequireAuth(verifier TokenVerifier, logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				unauthorized(w, "missing bearer token")
				return
			}

			claims, err := verifier.Verify(r.Context(), token)
			if err != nil {
				// The reason goes to the log, not the response: which check
				// failed is useful to an attacker probing tokens, and useless
				// to an honest client, whose only move either way is to get a
				// fresh token. Operators still need it to tell a misconfigured
				// issuer from an actual attack.
				logger.Warn().
					Err(err).
					Str("path", r.URL.Path).
					Str("request_id", middleware.GetReqID(r.Context())).
					Msg("rejected token")
				unauthorized(w, "invalid token")
				return
			}

			next.ServeHTTP(w, r.WithContext(auth.NewContext(r.Context(), claims)))
		})
	}
}

// invitePrefix is where the accept route lives. An invite token travels in
// that URL and is a bearer credential, so the path is the one field that has to
// be edited before it is written down.
const invitePrefix = "/api/invites/"

// logPath strips the invite token out of a request path. A log line is exactly
// the place a token must not end up: anyone reading the log could otherwise
// join the workspace it was issued for.
func logPath(path string) string {
	if strings.HasPrefix(path, invitePrefix) {
		return invitePrefix + "{token}/accept"
	}
	return path
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// handleMe echoes the caller's identity, which is how the frontend confirms a
// token is good without guessing at the backend's rules.
func handleMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		// Only reachable if this handler is mounted without RequireAuth.
		writeError(w, r, auth.ErrNoIdentity)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":    claims.Subject,
		"email": claims.Email,
		"name":  claims.Name,
	})
}

// bearerToken pulls the credential out of an Authorization header, accepting
// the scheme case-insensitively as RFC 7235 requires.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func unauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	writeJSON(w, http.StatusUnauthorized, map[string]any{"error": message})
}

// maxBodyBytes caps a request body. Most endpoints here take a handful of short
// fields, and an unbounded decode is memory the caller chooses.
const maxBodyBytes = 64 << 10

// decodeJSON reads a JSON body into v, refusing unknown fields so a misspelled
// key is a 400 rather than a value that is quietly ignored.
func decodeJSON(r *http.Request, v any) error {
	return decodeJSONLimit(r, v, maxBodyBytes)
}

// decodeJSONLimit is decodeJSON with a different cap, for the endpoints whose
// bodies are legitimately large — a workflow graph carries node configs, so the
// default would reject real work.
func decodeJSONLimit(r *http.Request, v any, limit int64) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return invalid("request body: %v", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
