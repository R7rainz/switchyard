package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/workspace"
)

const storedSecret = "ghp_a_very_real_looking_token"

// credentialRouter wires a real credential service over in-memory storage, so
// the encryption runs for real and only the database is faked.
func credentialRouter(t *testing.T) (http.Handler, *workspace.Service) {
	t.Helper()

	key := make([]byte, credential.KeySize)
	for i := range key {
		key[i] = byte(i)
	}
	ring, err := credential.NewKeyring(1, map[int][]byte{1: key})
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}

	store := workspace.NewMemoryStore()
	workspaces := workspace.NewService(store)
	credentials := credential.NewService(credential.NewMemoryStore(), ring)

	router := NewRouter(tokenNamesTheCaller{}, testLogger(), workspaces, credentials, testAppURL)
	return router, workspaces
}

// rawCall is call's sibling that keeps the body as it went over the wire. The
// leak test has to search the actual bytes, not a decoded map that might have
// dropped the field carrying the secret.
func rawCall(t *testing.T, handler http.Handler, method, path, userID, body string) (int, string) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Authorization", "Bearer "+userID)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.String()
}

func TestCredentialRoundTripThroughHTTP(t *testing.T) {
	router, _ := credentialRouter(t)
	ws := firstWorkspace(t, router, "alice")

	status, _ := rawCall(t, router, http.MethodPut,
		"/api/workspaces/"+ws+"/credentials/github/ci", "alice",
		`{"secret":"`+storedSecret+`"}`)
	if status != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", status)
	}

	status, body := rawCall(t, router, http.MethodGet, "/api/workspaces/"+ws+"/credentials", "alice", "")
	if status != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", status)
	}
	if !strings.Contains(body, `"provider":"github"`) || !strings.Contains(body, `"name":"ci"`) {
		t.Errorf("listing = %s, want the credential's identity", body)
	}
}

// The point of the whole package: a stored secret has no way back out over
// HTTP. If this ever fails, encryption at rest has been undone by an endpoint.
func TestNoEndpointReturnsTheSecret(t *testing.T) {
	router, _ := credentialRouter(t)
	ws := firstWorkspace(t, router, "alice")

	if status, _ := rawCall(t, router, http.MethodPut,
		"/api/workspaces/"+ws+"/credentials/github/ci", "alice",
		`{"secret":"`+storedSecret+`"}`); status != http.StatusNoContent {
		t.Fatalf("PUT status = %d", status)
	}

	// Every route that could plausibly carry it back.
	for _, path := range []string{
		"/api/workspaces/" + ws + "/credentials",
		"/api/workspaces/" + ws + "/credentials/github/ci",
		"/api/workspaces/" + ws,
		"/api/workspaces",
	} {
		_, body := rawCall(t, router, http.MethodGet, path, "alice", "")
		if strings.Contains(body, storedSecret) {
			t.Errorf("GET %s leaked the secret: %s", path, body)
		}
	}

	// And the write itself must not echo it.
	_, body := rawCall(t, router, http.MethodPut,
		"/api/workspaces/"+ws+"/credentials/github/other", "alice",
		`{"secret":"`+storedSecret+`"}`)
	if strings.Contains(body, storedSecret) {
		t.Errorf("PUT echoed the secret: %s", body)
	}
}

func TestPutCredentialReplacesRatherThanDuplicating(t *testing.T) {
	router, _ := credentialRouter(t)
	ws := firstWorkspace(t, router, "alice")

	for _, secret := range []string{"first", "second"} {
		if status, _ := rawCall(t, router, http.MethodPut,
			"/api/workspaces/"+ws+"/credentials/github/ci", "alice",
			`{"secret":"`+secret+`"}`); status != http.StatusNoContent {
			t.Fatalf("PUT %s: status %d", secret, status)
		}
	}

	_, body := rawCall(t, router, http.MethodGet, "/api/workspaces/"+ws+"/credentials", "alice", "")
	var listing struct {
		Credentials []credentialView `json:"credentials"`
	}
	if err := json.Unmarshal([]byte(body), &listing); err != nil {
		t.Fatalf("decoding listing: %v", err)
	}
	if len(listing.Credentials) != 1 {
		t.Errorf("workspace holds %d credentials, want 1 — PUT is not idempotent", len(listing.Credentials))
	}
}

func TestDeleteCredential(t *testing.T) {
	router, _ := credentialRouter(t)
	ws := firstWorkspace(t, router, "alice")

	rawCall(t, router, http.MethodPut, "/api/workspaces/"+ws+"/credentials/github/ci", "alice",
		`{"secret":"`+storedSecret+`"}`)

	if status, _ := rawCall(t, router, http.MethodDelete,
		"/api/workspaces/"+ws+"/credentials/github/ci", "alice", ""); status != http.StatusNoContent {
		t.Errorf("DELETE status = %d, want 204", status)
	}
	// Deleting what is not there is a 404, not silence.
	if status, _ := rawCall(t, router, http.MethodDelete,
		"/api/workspaces/"+ws+"/credentials/github/ci", "alice", ""); status != http.StatusNotFound {
		t.Errorf("second DELETE status = %d, want 404", status)
	}
}

func TestCredentialsNeedAdmin(t *testing.T) {
	// A member runs workflows that use the keys; deciding which keys exist is
	// a different thing. The listing alone reveals which providers a
	// workspace is wired to.
	router, workspaces := credentialRouter(t)
	ws := firstWorkspace(t, router, "alice")

	ctx := context.Background()
	for user, role := range map[string]auth.Role{"bob": auth.RoleMember, "carol": auth.RoleAdmin} {
		_, token, err := workspaces.Invite(ctx, ws, "alice", workspace.InviteRequest{Role: role, MaxUses: 5})
		if err != nil {
			t.Fatalf("Invite: %v", err)
		}
		if _, err := workspaces.Accept(ctx, token, user); err != nil {
			t.Fatalf("Accept for %s: %v", user, err)
		}
	}

	if status, _ := rawCall(t, router, http.MethodGet, "/api/workspaces/"+ws+"/credentials", "bob", ""); status != http.StatusForbidden {
		t.Errorf("member listing credentials = %d, want 403", status)
	}
	if status, _ := rawCall(t, router, http.MethodPut,
		"/api/workspaces/"+ws+"/credentials/github/ci", "bob", `{"secret":"x"}`); status != http.StatusForbidden {
		t.Errorf("member writing a credential = %d, want 403", status)
	}
	if status, _ := rawCall(t, router, http.MethodGet, "/api/workspaces/"+ws+"/credentials", "carol", ""); status != http.StatusOK {
		t.Errorf("admin listing credentials = %d, want 200", status)
	}

	// A stranger still cannot tell the workspace exists.
	if status, _ := rawCall(t, router, http.MethodGet, "/api/workspaces/"+ws+"/credentials", "stranger", ""); status != http.StatusNotFound {
		t.Errorf("stranger listing credentials = %d, want 404", status)
	}
}

func TestCredentialsAreScopedToTheirWorkspace(t *testing.T) {
	router, _ := credentialRouter(t)
	alice := firstWorkspace(t, router, "alice")
	bob := firstWorkspace(t, router, "bob")

	rawCall(t, router, http.MethodPut, "/api/workspaces/"+alice+"/credentials/github/ci", "alice",
		`{"secret":"`+storedSecret+`"}`)

	_, body := rawCall(t, router, http.MethodGet, "/api/workspaces/"+bob+"/credentials", "bob", "")
	if strings.Contains(body, "github") {
		t.Errorf("bob's workspace lists alice's credential: %s", body)
	}
}

func TestPutCredentialRejectsBadInput(t *testing.T) {
	router, _ := credentialRouter(t)
	ws := firstWorkspace(t, router, "alice")

	tests := map[string]string{
		"empty secret":   `{"secret":""}`,
		"missing secret": `{}`,
		"unknown field":  `{"secret":"x","extra":true}`,
		"not json":       `not json at all`,
		"oversized":      `{"secret":"` + strings.Repeat("x", maxSecretBytes+1) + `"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			status, _ := rawCall(t, router, http.MethodPut,
				"/api/workspaces/"+ws+"/credentials/github/ci", "alice", body)
			if status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", status)
			}
		})
	}
}
