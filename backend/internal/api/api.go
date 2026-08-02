package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/auth"
)

// TokenVerifier is the part of the auth package this layer needs. It is
// declared here, where it is consumed, so handlers can be tested without a
// live JWKS endpoint.
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (*auth.Claims, error)
}

// NewRouter builds the HTTP surface. Routing, encoding, and auth enforcement
// live here; everything else belongs to the domain packages.
func NewRouter(verifier TokenVerifier, logger zerolog.Logger) http.Handler {
	router := chi.NewRouter()

	// RequestID first, so every line the other middleware logs can be tied
	// back to one request.
	router.Use(middleware.RequestID)
	router.Use(RequestLogger(logger))
	// A panic in one handler should fail one request, not the process.
	router.Use(middleware.Recoverer)

	// Unauthenticated: this is what a load balancer polls.
	router.Get("/healthz", handleHealthz)

	router.Route("/api", func(r chi.Router) {
		r.Use(RequireAuth(verifier, logger))
		r.Get("/me", handleMe)
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
				Str("path", r.URL.Path).
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
