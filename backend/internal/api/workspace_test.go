package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/workspace"
)

// testAppURL stands in for the frontend, which is where an invite link points.
const testAppURL = "http://localhost:3007"

// testRouter builds the production router over in-memory storage, so a request
// meets the same middleware chain it would in the server.
func testRouter(verifier TokenVerifier, logger zerolog.Logger) http.Handler {
	store := workspace.NewMemoryStore()
	return NewRouter(verifier, logger, workspace.NewService(store), nil, nil, nil, testAppURL)
}

// mount builds a router with one gated route, backed by a real workspace
// service so the middleware is tested against the rules it will actually meet.
func mount(t *testing.T, svc *workspace.Service, p auth.Permission) http.Handler {
	t.Helper()

	router := chi.NewRouter()
	router.Route("/api/workspaces/{workspaceID}", func(r chi.Router) {
		r.Use(RequirePermission(svc, p))
		r.Get("/thing", func(w http.ResponseWriter, r *http.Request) {
			role, _ := WorkspaceRole(r.Context())
			writeJSON(w, http.StatusOK, map[string]any{"role": string(role)})
		})
	})
	return router
}

// request issues a GET as userID, with the logger the error path needs.
func request(handler http.Handler, workspaceID, userID string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+workspaceID+"/thing", nil)
	ctx := r.Context()
	if userID != "" {
		ctx = auth.NewContext(ctx, &auth.Claims{Subject: userID})
	}
	ctx = zerolog.New(nil).WithContext(ctx)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, r.WithContext(ctx))
	return recorder
}

func newWorkspace(t *testing.T) (*workspace.Service, string) {
	t.Helper()
	svc := workspace.NewService(workspace.NewMemoryStore())
	created, err := svc.Create(context.Background(), "owner-1", "Test", "test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return svc, created.ID
}

func TestRequirePermissionAllowsAMemberWithTheRole(t *testing.T) {
	svc, workspaceID := newWorkspace(t)
	handler := mount(t, svc, auth.PermissionWorkflowWrite)

	recorder := request(handler, workspaceID, "owner-1")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body)
	}
	// The handler below can see the role without looking it up again.
	if body := recorder.Body.String(); !strings.Contains(body, "OWNER") {
		t.Errorf("body = %q, want it to carry the caller's role", body)
	}
}

func TestRequirePermissionRejects(t *testing.T) {
	svc, workspaceID := newWorkspace(t)
	ctx := context.Background()

	// A viewer is a member, but not one who may write.
	if _, token, err := svc.Invite(ctx, workspaceID, "owner-1", workspace.InviteRequest{Role: auth.RoleViewer, MaxUses: 5}); err != nil {
		t.Fatalf("Invite: %v", err)
	} else if _, err := svc.Accept(ctx, token, "viewer-1"); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	tests := []struct {
		name       string
		userID     string
		wantStatus int
	}{
		{name: "unauthenticated", userID: "", wantStatus: http.StatusUnauthorized},
		// A stranger gets 404, not 403 — otherwise the id is confirmed real.
		{name: "not a member", userID: "stranger", wantStatus: http.StatusNotFound},
		// A member already knows the workspace exists, so 403 leaks nothing.
		{name: "role too low", userID: "viewer-1", wantStatus: http.StatusForbidden},
	}

	handler := mount(t, svc, auth.PermissionWorkflowWrite)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := request(handler, workspaceID, tc.userID).Code; got != tc.wantStatus {
				t.Errorf("status = %d, want %d", got, tc.wantStatus)
			}
		})
	}
}

func TestUnknownWorkspaceLooksLikeANonMembership(t *testing.T) {
	// Both answer 404, so an authenticated caller cannot tell a workspace they
	// are not in from one that does not exist.
	svc, workspaceID := newWorkspace(t)
	handler := mount(t, svc, auth.PermissionWorkflowRead)

	missing := request(handler, "does-not-exist", "owner-1")
	stranger := request(handler, workspaceID, "stranger")

	if missing.Code != http.StatusNotFound || stranger.Code != http.StatusNotFound {
		t.Fatalf("missing = %d, stranger = %d, want both 404", missing.Code, stranger.Code)
	}
	if missing.Body.String() != stranger.Body.String() {
		t.Errorf("bodies differ:\n missing  = %s\n stranger = %s", missing.Body, stranger.Body)
	}
}

func TestRoleChangeTakesEffectWithoutANewToken(t *testing.T) {
	// The check is per request, not off the token, so a demotion applies
	// immediately rather than when the 15-minute JWT expires.
	svc, workspaceID := newWorkspace(t)
	ctx := context.Background()

	_, token, err := svc.Invite(ctx, workspaceID, "owner-1", workspace.InviteRequest{Role: auth.RoleAdmin})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if _, err := svc.Accept(ctx, token, "admin-1"); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	handler := mount(t, svc, auth.PermissionCredentialManage)
	if got := request(handler, workspaceID, "admin-1").Code; got != http.StatusOK {
		t.Fatalf("status = %d, want 200 while admin", got)
	}

	if err := svc.SetRole(ctx, workspaceID, "owner-1", "admin-1", auth.RoleViewer); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if got := request(handler, workspaceID, "admin-1").Code; got != http.StatusForbidden {
		t.Errorf("status = %d after demotion, want 403", got)
	}
}

func TestRouteWithoutWorkspaceParameterFailsClosed(t *testing.T) {
	// Mounting the middleware where no {workspaceID} exists is a wiring bug.
	// It must not be read as "no workspace, so allow".
	svc, _ := newWorkspace(t)

	router := chi.NewRouter()
	router.With(RequirePermission(svc, auth.PermissionWorkflowRead)).
		Get("/oops", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"reached": true})
		})

	r := httptest.NewRequest(http.MethodGet, "/oops", nil)
	ctx := auth.NewContext(r.Context(), &auth.Claims{Subject: "owner-1"})
	ctx = zerolog.New(nil).WithContext(ctx)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, r.WithContext(ctx))

	if recorder.Code == http.StatusOK {
		t.Error("a route with no workspace parameter let the request through")
	}
}

// tokenNamesTheCaller reads a bearer token as the id of whoever presents it, so
// these tests can act as several people without minting real JWTs.
type tokenNamesTheCaller struct{}

func (tokenNamesTheCaller) Verify(_ context.Context, token string) (*auth.Claims, error) {
	if token == "" {
		return nil, errors.New("auth: empty token")
	}
	return &auth.Claims{Subject: token, Email: token + "@switchyard.test", Name: token}, nil
}

// routes builds the whole API over an empty in-memory store.
func routes() http.Handler {
	store := workspace.NewMemoryStore()
	return NewRouter(tokenNamesTheCaller{}, testLogger(), workspace.NewService(store), nil, nil, nil, testAppURL)
}

// call issues an authenticated request as userID and decodes the response.
func call(t *testing.T, handler http.Handler, method, path, userID, body string) (int, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Authorization", "Bearer "+userID)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	decoded := map[string]any{}
	if recorder.Body.Len() > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s %s as %s: body %q is not a JSON object", method, path, userID, recorder.Body)
		}
	}
	return recorder.Code, decoded
}

// field reads a string out of a decoded body, failing rather than panicking
// when the shape is not the one the test expects.
func field(t *testing.T, body map[string]any, key string) string {
	t.Helper()
	value, ok := body[key].(string)
	if !ok {
		t.Fatalf("%q missing from %v", key, body)
	}
	return value
}

// firstWorkspace lists as userID and returns their first workspace, which for a
// new account is the one the listing just created.
func firstWorkspace(t *testing.T, handler http.Handler, userID string) string {
	t.Helper()

	status, body := call(t, handler, http.MethodGet, "/api/workspaces", userID, "")
	if status != http.StatusOK {
		t.Fatalf("listing workspaces as %s: status = %d", userID, status)
	}
	found, _ := body["workspaces"].([]any)
	if len(found) == 0 {
		t.Fatalf("%s has no workspace: %v", userID, body)
	}
	first, _ := found[0].(map[string]any)
	return field(t, first, "id")
}

func TestFirstRequestGivesAnAccountAWorkspace(t *testing.T) {
	// A Better Auth signup creates a user and nothing else, so without this
	// every workspace-scoped call would 404 for the life of the account.
	api := routes()

	workspaceID := firstWorkspace(t, api, "alice")

	// Listing again returns the same one rather than making another.
	status, body := call(t, api, http.MethodGet, "/api/workspaces", "alice", "")
	found, _ := body["workspaces"].([]any)
	if status != http.StatusOK || len(found) != 1 {
		t.Fatalf("second listing: status = %d, workspaces = %d, want 200 and 1", status, len(found))
	}
	if again, _ := found[0].(map[string]any); field(t, again, "id") != workspaceID {
		t.Error("the second listing returned a different workspace")
	}

	// And the account owns it, or it could not invite anyone into it.
	_, members := call(t, api, http.MethodGet, "/api/workspaces/"+workspaceID+"/members", "alice", "")
	if listed := toJSON(t, members["members"]); !strings.Contains(listed, `"role":"OWNER"`) {
		t.Errorf("members = %s, want alice as OWNER", listed)
	}
}

func TestInviteAcceptAndMemberAppears(t *testing.T) {
	api := routes()
	workspaceID := firstWorkspace(t, api, "alice")

	status, created := call(t, api, http.MethodPost, "/api/workspaces/"+workspaceID+"/invites", "alice",
		`{"email":"bob@switchyard.test","role":"MEMBER"}`)
	if status != http.StatusCreated {
		t.Fatalf("creating an invite: status = %d, want 201 (%v)", status, created)
	}

	token := field(t, created, "token")
	if got, want := field(t, created, "joinURL"), testAppURL+"/invite/"+token; got != want {
		t.Errorf("joinURL = %q, want %q", got, want)
	}

	// Bob is not a member yet, which is exactly why accepting cannot be
	// workspace-scoped.
	if status, _ := call(t, api, http.MethodGet, "/api/workspaces/"+workspaceID+"/members", "bob", ""); status != http.StatusNotFound {
		t.Fatalf("status = %d before accepting, want 404", status)
	}

	status, joined := call(t, api, http.MethodPost, "/api/invites/"+token+"/accept", "bob", "")
	if status != http.StatusOK {
		t.Fatalf("accepting: status = %d, want 200 (%v)", status, joined)
	}
	if got := field(t, joined, "id"); got != workspaceID {
		t.Errorf("accepting returned workspace %q, want %q", got, workspaceID)
	}

	status, members := call(t, api, http.MethodGet, "/api/workspaces/"+workspaceID+"/members", "bob", "")
	if status != http.StatusOK {
		t.Fatalf("listing members as bob: status = %d, want 200", status)
	}
	if listed := toJSON(t, members["members"]); !strings.Contains(listed, `"userId":"bob"`) || !strings.Contains(listed, `"role":"MEMBER"`) {
		t.Errorf("members = %s, want bob as a MEMBER", listed)
	}
}

func TestViewerCannotInvite(t *testing.T) {
	api := routes()
	workspaceID := firstWorkspace(t, api, "alice")

	_, created := call(t, api, http.MethodPost, "/api/workspaces/"+workspaceID+"/invites", "alice", `{"role":"VIEWER"}`)
	if status, _ := call(t, api, http.MethodPost, "/api/invites/"+field(t, created, "token")+"/accept", "carol", ""); status != http.StatusOK {
		t.Fatalf("carol could not accept: status = %d", status)
	}

	// A member already knows the workspace exists, so 403 leaks nothing.
	status, body := call(t, api, http.MethodPost, "/api/workspaces/"+workspaceID+"/invites", "carol", `{"role":"VIEWER"}`)
	if status != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (%v)", status, body)
	}
}

func TestAViewerCanLeaveButCannotRemoveAnyoneElse(t *testing.T) {
	// This is why DELETE on a member is gated on membership rather than
	// member:manage: leaving is not an act of authority, and a viewer invited
	// by mistake would otherwise be stuck.
	api := routes()
	workspaceID := firstWorkspace(t, api, "alice")

	_, created := call(t, api, http.MethodPost, "/api/workspaces/"+workspaceID+"/invites", "alice", `{"role":"VIEWER","maxUses":2}`)
	token := field(t, created, "token")
	for _, who := range []string{"carol", "dave"} {
		if status, _ := call(t, api, http.MethodPost, "/api/invites/"+token+"/accept", who, ""); status != http.StatusOK {
			t.Fatalf("%s could not accept: status = %d", who, status)
		}
	}

	if status, body := call(t, api, http.MethodDelete, "/api/workspaces/"+workspaceID+"/members/dave", "carol", ""); status != http.StatusForbidden {
		t.Errorf("carol removing dave: status = %d, want 403 (%v)", status, body)
	}
	if status, body := call(t, api, http.MethodDelete, "/api/workspaces/"+workspaceID+"/members/carol", "carol", ""); status != http.StatusNoContent {
		t.Fatalf("carol leaving: status = %d, want 204 (%v)", status, body)
	}
	if status, _ := call(t, api, http.MethodGet, "/api/workspaces/"+workspaceID+"/members", "carol", ""); status != http.StatusNotFound {
		t.Error("carol still has standing in the workspace she left")
	}
}

func TestStrangerCannotTellWorkspacesApart(t *testing.T) {
	// 404 rather than 403: a stranger must not learn that an id is real.
	api := routes()
	workspaceID := firstWorkspace(t, api, "alice")

	real := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+workspaceID+"/members", nil)
	request.Header.Set("Authorization", "Bearer mallory")
	api.ServeHTTP(real, request)

	missing := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/workspaces/does-not-exist/members", nil)
	request.Header.Set("Authorization", "Bearer mallory")
	api.ServeHTTP(missing, request)

	if real.Code != http.StatusNotFound || missing.Code != http.StatusNotFound {
		t.Fatalf("real = %d, missing = %d, want both 404", real.Code, missing.Code)
	}
	if real.Body.String() != missing.Body.String() {
		t.Errorf("bodies differ:\n real    = %s\n missing = %s", real.Body, missing.Body)
	}
}

func TestInviteTokenIsReturnedOnceAndNeverListed(t *testing.T) {
	api := routes()
	workspaceID := firstWorkspace(t, api, "alice")

	_, created := call(t, api, http.MethodPost, "/api/workspaces/"+workspaceID+"/invites", "alice", `{"role":"ADMIN","maxUses":3}`)
	token := field(t, created, "token")
	if token == "" {
		t.Fatal("the create response carried no token")
	}

	status, listed := call(t, api, http.MethodGet, "/api/workspaces/"+workspaceID+"/invites", "alice", "")
	if status != http.StatusOK {
		t.Fatalf("listing invites: status = %d, want 200", status)
	}

	body := toJSON(t, listed)
	if strings.Contains(body, token) {
		t.Error("the listing returned the invite token")
	}
	// Nor the stored hash, which would let a guess be confirmed offline.
	if strings.Contains(body, workspace.HashToken(token)) || strings.Contains(body, "tokenHash") {
		t.Errorf("the listing returned the token hash: %s", body)
	}
}

func TestMalformedBodiesAreRejected(t *testing.T) {
	api := routes()
	workspaceID := firstWorkspace(t, api, "alice")
	invites := "/api/workspaces/" + workspaceID + "/invites"

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "not json", method: http.MethodPost, path: "/api/workspaces", body: `{`},
		{name: "no name", method: http.MethodPost, path: "/api/workspaces", body: `{"name":"   "}`},
		{name: "name too long", method: http.MethodPost, path: "/api/workspaces", body: `{"name":"` + strings.Repeat("a", maxNameLen+1) + `"}`},
		{name: "unknown field", method: http.MethodPost, path: "/api/workspaces", body: `{"name":"Fine","owner":"me"}`},
		{name: "name with nothing sluggable", method: http.MethodPost, path: "/api/workspaces", body: `{"name":"???"}`},
		{name: "unknown role", method: http.MethodPost, path: invites, body: `{"role":"SUPERUSER"}`},
		{name: "no role", method: http.MethodPost, path: invites, body: `{"email":"bob@switchyard.test"}`},
		{name: "negative uses", method: http.MethodPost, path: invites, body: `{"role":"MEMBER","maxUses":-1}`},
		{name: "email that is not one", method: http.MethodPost, path: invites, body: `{"role":"MEMBER","email":"bob"}`},
		{name: "empty role", method: http.MethodPatch, path: "/api/workspaces/" + workspaceID + "/members/alice", body: `{"role":""}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, body := call(t, api, tc.method, tc.path, "alice", tc.body)
			if status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (%v)", status, body)
			}
		})
	}
}

func TestInviteTokenStaysOutOfTheLog(t *testing.T) {
	// The token rides in the URL, and the request log is exactly where a bearer
	// credential must not end up.
	var buf bytes.Buffer

	store := workspace.NewMemoryStore()
	api := NewRouter(tokenNamesTheCaller{}, captureLogs(&buf, zerolog.InfoLevel),
		workspace.NewService(store), nil, nil, nil, testAppURL)

	workspaceID := firstWorkspace(t, api, "alice")
	_, created := call(t, api, http.MethodPost, "/api/workspaces/"+workspaceID+"/invites", "alice", `{"role":"MEMBER"}`)
	token := field(t, created, "token")

	call(t, api, http.MethodPost, "/api/invites/"+token+"/accept", "bob", "")

	if strings.Contains(buf.String(), token) {
		t.Errorf("the invite token was written to the log: %s", buf.String())
	}
}

func toJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("re-encoding %v: %v", value, err)
	}
	return string(encoded)
}
