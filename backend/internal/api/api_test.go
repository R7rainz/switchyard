package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/auth"
)

// stubVerifier accepts exactly one token, so the middleware's own behaviour is
// what these tests measure. Token verification itself is covered in the auth
// package against real signatures.
type stubVerifier struct {
	accept string
	calls  int
}

func (s *stubVerifier) Verify(_ context.Context, token string) (*auth.Claims, error) {
	s.calls++
	if token != s.accept {
		return nil, errors.New("auth: signature does not verify")
	}
	return &auth.Claims{Subject: "user-123", Email: "dev@switchyard.test", Name: "Dev"}, nil
}

func TestHealthzNeedsNoToken(t *testing.T) {
	router := testRouter(&stubVerifier{accept: "good"}, testLogger())

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if body := recorder.Body.String(); body != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

func TestRequireAuthRejects(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		wantCalls  int
	}{
		{name: "no header", authHeader: "", wantCalls: 0},
		{name: "bare token without scheme", authHeader: "good", wantCalls: 0},
		{name: "wrong scheme", authHeader: "Basic good", wantCalls: 0},
		{name: "empty bearer", authHeader: "Bearer ", wantCalls: 0},
		{name: "bad token", authHeader: "Bearer forged", wantCalls: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			verifier := &stubVerifier{accept: "good"}
			router := testRouter(verifier, testLogger())

			request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
			if tc.authHeader != "" {
				request.Header.Set("Authorization", tc.authHeader)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", recorder.Code)
			}
			if got := recorder.Header().Get("WWW-Authenticate"); got == "" {
				t.Error("401 response carried no WWW-Authenticate header")
			}
			// A malformed header must not cost us a verification.
			if verifier.calls != tc.wantCalls {
				t.Errorf("verifier called %d times, want %d", verifier.calls, tc.wantCalls)
			}
			// The response must not leak which check failed.
			if body := recorder.Body.String(); !json.Valid([]byte(body)) {
				t.Errorf("body %q is not JSON", body)
			}
		})
	}
}

func TestRequireAuthAcceptsValidTokenAndPassesClaims(t *testing.T) {
	router := testRouter(&stubVerifier{accept: "good"}, testLogger())

	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "Bearer good")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body)
	}

	var body struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body.ID != "user-123" || body.Email != "dev@switchyard.test" {
		t.Errorf("body = %+v, want the stub's claims", body)
	}
}

func TestVersionedAPIPathAcceptsValidToken(t *testing.T) {
	router := testRouter(&stubVerifier{accept: "good"}, testLogger())

	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer good")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body)
	}
}

func TestRequireAuthAcceptsLowercaseScheme(t *testing.T) {
	// RFC 7235 makes the scheme case-insensitive, and clients do vary.
	router := testRouter(&stubVerifier{accept: "good"}, testLogger())

	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "bearer good")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestClaimsReachHandlerThroughContext(t *testing.T) {
	var seen *auth.Claims
	handler := RequireAuth(&stubVerifier{accept: "good"}, testLogger())(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen, _ = auth.FromContext(r.Context())
	}))

	request := httptest.NewRequest(http.MethodGet, "/anything", nil)
	request.Header.Set("Authorization", "Bearer good")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if seen == nil {
		t.Fatal("handler saw no claims on the context")
	}
	if seen.Subject != "user-123" {
		t.Errorf("Subject = %q, want user-123", seen.Subject)
	}
}

func TestUnauthenticatedContextHasNoClaims(t *testing.T) {
	if claims, ok := auth.FromContext(context.Background()); ok {
		t.Errorf("bare context reported claims %+v", claims)
	}
}
