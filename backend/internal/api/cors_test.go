package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// frontendOrigin is the only origin the router allows; it matches testAppURL,
// which is what main passes as the app URL.
const frontendOrigin = testAppURL

func corsRouter(t *testing.T) http.Handler {
	t.Helper()
	return testRouter(&stubVerifier{accept: "good"}, testLogger())
}

func TestPreflightIsAnsweredWithoutAToken(t *testing.T) {
	// The browser sends OPTIONS with no Authorization header. If RequireAuth
	// answered it with a 401 the real request would never leave the browser,
	// and the failure would look like CORS being misconfigured rather than
	// auth being in the wrong order.
	router := corsRouter(t)

	request := httptest.NewRequest(http.MethodOptions, "/api/workspaces", nil)
	request.Header.Set("Origin", frontendOrigin)
	request.Header.Set("Access-Control-Request-Method", "GET")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != frontendOrigin {
		t.Errorf("Allow-Origin = %q, want %q", got, frontendOrigin)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Errorf("Allow-Headers = %q, want it to permit Authorization", got)
	}
}

func TestAnotherOriginIsNotAllowed(t *testing.T) {
	// A wildcard would let any page drive this API with a token it obtained.
	router := corsRouter(t)

	request := httptest.NewRequest(http.MethodOptions, "/api/workspaces", nil)
	request.Header.Set("Origin", "https://evil.example")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q for a foreign origin, want it absent", got)
	}
}

func TestResponsesVaryOnOrigin(t *testing.T) {
	// Without Vary a cache could hand one origin's allowed response to another.
	router := corsRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("Origin", frontendOrigin)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Errorf("Vary = %q, want it to include Origin", got)
	}
}
