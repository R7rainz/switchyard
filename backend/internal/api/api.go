package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

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
func NewRouter(verifier TokenVerifier) http.Handler {
	router := chi.NewRouter()

	// A panic in one handler should fail one request, not the process.
	router.Use(middleware.Recoverer)

	// Unauthenticated: this is what a load balancer polls.
	router.Get("/healthz", handleHealthz)

	router.Route("/api", func(r chi.Router) {
		r.Use(RequireAuth(verifier))
		r.Get("/me", handleMe)
	})

	return router
}

// RequireAuth returns middleware that rejects any request without a valid
// Better Auth bearer token, and puts the verified claims on the request
// context for the handlers below it.
func RequireAuth(verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				unauthorized(w, "missing bearer token")
				return
			}

			claims, err := verifier.Verify(r.Context(), token)
			if err != nil {
				// The reason stays server-side: which check failed is useful to
				// an attacker probing tokens, and useless to an honest client,
				// whose only move either way is to get a fresh token.
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
		unauthorized(w, "invalid token")
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
