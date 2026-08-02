package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/workspace"
)

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
