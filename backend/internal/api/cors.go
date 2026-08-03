package api

import (
	"net/http"
)

// CORS allows the frontend's origin to call this API from a browser.
//
// The frontend runs on a different port to the backend, so every call from a
// page is cross-origin and a browser refuses it unless the server says
// otherwise. It is the reason a token that works from curl fails from the app.
//
// Exactly one origin is allowed — the app's own URL, which is already the JWT
// issuer, so there is no second thing to keep in step. `*` is not used and
// should not be: it would let any page on the internet drive this API with a
// token it managed to obtain.
func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Vary regardless of whether the origin matched: caches must not
			// serve one origin's response to another.
			w.Header().Add("Vary", "Origin")

			if origin != "" && origin == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				// Authorization is not a header a browser sends cross-origin
				// without being told it may.
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				// Cache the preflight so a browser does not re-ask before every
				// call it makes.
				w.Header().Set("Access-Control-Max-Age", "600")
			}

			// A preflight carries no credentials and must be answered before
			// any auth middleware sees it — a 401 here reads to the browser as
			// "not allowed" and the real request is never sent.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
