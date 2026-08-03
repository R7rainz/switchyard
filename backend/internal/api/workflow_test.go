package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
	"github.com/R7rainz/switchyard/backend/internal/workspace"
)

// smallGraph is the shape React Flow holds in useNodesState and useEdgesState,
// sent verbatim. If this ever needs rewriting to reach the API, the frontend
// has a mapping layer it should not have.
const smallGraph = `{
	"nodes": [
		{"id": "t", "type": "trigger.manual", "position": {"x": 0, "y": 0}, "data": {"label": "Manually"}},
		{"id": "a", "type": "http.request", "position": {"x": 200, "y": 0}, "data": {"label": "Call the API"}}
	],
	"edges": [{"id": "e1", "source": "t", "target": "a"}]
}`

// workflowRouter wires the real workflow service over in-memory storage, so
// validation runs for real and only the database is faked.
func workflowRouter(t *testing.T) (http.Handler, *workspace.Service) {
	t.Helper()

	workspaces := workspace.NewService(workspace.NewMemoryStore())
	workflows := workflow.NewService(workflow.NewMemoryStore())
	router := NewRouter(tokenNamesTheCaller{}, testLogger(), workspaces, nil, workflows, testAppURL)
	return router, workspaces
}

func TestWorkflowRoundTripThroughHTTP(t *testing.T) {
	router, _ := workflowRouter(t)
	ws := firstWorkspace(t, router, "alice")
	base := "/api/workspaces/" + ws + "/workflows"

	status, created := call(t, router, http.MethodPost, base, "alice",
		`{"name":"deploy on merge","description":"ships main","graph":`+smallGraph+`}`)
	if status != http.StatusCreated {
		t.Fatalf("create: status = %d, body %v", status, created)
	}
	id := field(t, created, "id")
	if field(t, created, "createdBy") != "alice" {
		t.Fatalf("createdBy = %v, want alice", created["createdBy"])
	}

	status, got := call(t, router, http.MethodGet, base+"/"+id, "alice", "")
	if status != http.StatusOK {
		t.Fatalf("get: status = %d, body %v", status, got)
	}
	graph, _ := got["graph"].(map[string]any)
	nodes, _ := graph["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("graph did not survive the round trip: %v", got["graph"])
	}

	status, listed := call(t, router, http.MethodGet, base, "alice", "")
	if status != http.StatusOK {
		t.Fatalf("list: status = %d", status)
	}
	if found, _ := listed["workflows"].([]any); len(found) != 1 {
		t.Fatalf("list returned %v, want one workflow", listed)
	}

	status, _ = call(t, router, http.MethodDelete, base+"/"+id, "alice", "")
	if status != http.StatusNoContent {
		t.Fatalf("delete: status = %d", status)
	}
	// Deleting the same thing twice is a 404, not a 500 — the second call is a
	// refresh-and-click-again, not a server fault.
	if status, _ = call(t, router, http.MethodDelete, base+"/"+id, "alice", ""); status != http.StatusNotFound {
		t.Fatalf("second delete: status = %d, want 404", status)
	}
}

// A corrupt graph is a 400 carrying the reason, not a 500 and not a bare "bad
// request".
func TestCorruptGraphIsExplained(t *testing.T) {
	router, _ := workflowRouter(t)
	ws := firstWorkspace(t, router, "alice")
	base := "/api/workspaces/" + ws + "/workflows"

	cases := map[string]struct{ graph, mentions string }{
		"dangling edge": {
			`{"nodes":[{"id":"t","type":"trigger.manual"}],"edges":[{"id":"e1","source":"t","target":"ghost"}]}`,
			"ghost",
		},
		"duplicate node id": {
			`{"nodes":[{"id":"t","type":"trigger.manual"},{"id":"t","type":"http.request"}],"edges":[]}`,
			"duplicate node id",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			status, body := call(t, router, http.MethodPost, base, "alice",
				`{"name":"bad","graph":`+tc.graph+`}`)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %v", status, body)
			}
			if message := field(t, body, "error"); !strings.Contains(message, tc.mentions) {
				t.Fatalf("error %q does not mention %q", message, tc.mentions)
			}
		})
	}
}

// The half-built states a React Flow canvas passes through all have to save,
// because the builder autosaves and the user is not finished yet. This is the
// test that would have caught the original design.
func TestBuilderDraftsSave(t *testing.T) {
	router, _ := workflowRouter(t)
	ws := firstWorkspace(t, router, "alice")
	base := "/api/workspaces/" + ws + "/workflows"

	drafts := map[string]string{
		"empty canvas":                             `{"nodes":[],"edges":[]}`,
		"one node just dropped":                    `{"nodes":[{"id":"n1","type":"http.request","position":{"x":10,"y":10}}],"edges":[]}`,
		"nodes not yet connected":                  `{"nodes":[{"id":"t","type":"trigger.manual"},{"id":"a","type":"slack.message"}],"edges":[]}`,
		"a node type the backend has not heard of": `{"nodes":[{"id":"n1","type":"kubernetes.apply"}],"edges":[]}`,
	}

	for name, graph := range drafts {
		t.Run(name, func(t *testing.T) {
			status, body := call(t, router, http.MethodPost, base, "alice",
				`{"name":"work in progress","graph":`+graph+`}`)
			if status != http.StatusCreated {
				t.Fatalf("a draft must save: status = %d, body %v", status, body)
			}
		})
	}
}

// PATCH must touch only what it names. Sent as HTTP because that is where an
// absent key and an empty one are easiest to confuse.
func TestPatchLeavesUnsentFieldsAlone(t *testing.T) {
	router, _ := workflowRouter(t)
	ws := firstWorkspace(t, router, "alice")
	base := "/api/workspaces/" + ws + "/workflows"

	_, created := call(t, router, http.MethodPost, base, "alice",
		`{"name":"original","description":"keep me","graph":`+smallGraph+`}`)
	id := field(t, created, "id")

	status, patched := call(t, router, http.MethodPatch, base+"/"+id, "alice", `{"name":"renamed"}`)
	if status != http.StatusOK {
		t.Fatalf("patch: status = %d, body %v", status, patched)
	}
	if field(t, patched, "name") != "renamed" {
		t.Fatalf("name = %v", patched["name"])
	}
	if field(t, patched, "description") != "keep me" {
		t.Fatalf("description = %v, want it untouched", patched["description"])
	}
	graph, _ := patched["graph"].(map[string]any)
	if nodes, _ := graph["nodes"].([]any); len(nodes) != 2 {
		t.Fatalf("graph changed on a rename: %v", patched["graph"])
	}
}

// A workflow id from another workspace must not resolve, even though the caller
// is a legitimate admin of the workspace in the URL. Permission was checked
// against that workspace; the lookup has to agree.
func TestWorkflowIsNotVisibleFromAnotherWorkspace(t *testing.T) {
	router, _ := workflowRouter(t)

	alice := firstWorkspace(t, router, "alice")
	bob := firstWorkspace(t, router, "bob")

	_, created := call(t, router, http.MethodPost, "/api/workspaces/"+alice+"/workflows", "alice",
		`{"name":"alice's","graph":`+smallGraph+`}`)
	id := field(t, created, "id")

	// Bob owns his own workspace, so this is not a permission failure — it is
	// a lookup that must miss.
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		status, _ := call(t, router, method, "/api/workspaces/"+bob+"/workflows/"+id, "bob", "")
		if status != http.StatusNotFound {
			t.Fatalf("%s from bob's workspace: status = %d, want 404", method, status)
		}
	}
	status, _ := call(t, router, http.MethodPatch, "/api/workspaces/"+bob+"/workflows/"+id, "bob", `{"name":"stolen"}`)
	if status != http.StatusNotFound {
		t.Fatalf("patch from bob's workspace: status = %d, want 404", status)
	}
}

// The permission table decides who may write; this checks the routes are hung
// off the right entries in it.
func TestWorkflowPermissions(t *testing.T) {
	router, workspaces := workflowRouter(t)
	ws := firstWorkspace(t, router, "alice")
	base := "/api/workspaces/" + ws + "/workflows"

	_, created := call(t, router, http.MethodPost, base, "alice", `{"name":"alice's","graph":`+smallGraph+`}`)
	id := field(t, created, "id")

	joinAs(t, workspaces, ws, "viewer", auth.RoleViewer)

	// A viewer reads.
	if status, _ := call(t, router, http.MethodGet, base, "viewer", ""); status != http.StatusOK {
		t.Fatalf("viewer listing: status = %d, want 200", status)
	}
	if status, _ := call(t, router, http.MethodGet, base+"/"+id, "viewer", ""); status != http.StatusOK {
		t.Fatalf("viewer reading: status = %d, want 200", status)
	}

	// A viewer does not write. 403 rather than 404: they already know the
	// workspace exists, so nothing is leaked by saying no plainly.
	writes := []struct {
		method, path, body string
	}{
		{http.MethodPost, base, `{"name":"nope","graph":` + smallGraph + `}`},
		{http.MethodPatch, base + "/" + id, `{"name":"nope"}`},
		{http.MethodDelete, base + "/" + id, ""},
	}
	for _, attempt := range writes {
		status, _ := call(t, router, attempt.method, attempt.path, "viewer", attempt.body)
		if status != http.StatusForbidden {
			t.Fatalf("viewer %s %s: status = %d, want 403", attempt.method, attempt.path, status)
		}
	}

	// A stranger gets 404 everywhere, so workspace ids cannot be probed.
	if status, _ := call(t, router, http.MethodGet, base, "stranger", ""); status != http.StatusNotFound {
		t.Fatalf("stranger listing: status = %d, want 404", status)
	}
}

// joinAs puts a user in a workspace through the real invite flow, since that is
// the only way in and faking it would test a path nobody uses.
func joinAs(t *testing.T, workspaces *workspace.Service, workspaceID, userID string, role auth.Role) {
	t.Helper()
	_, token, err := workspaces.Invite(t.Context(), workspaceID, "alice", workspace.InviteRequest{Role: role})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if _, err := workspaces.Accept(t.Context(), token, userID); err != nil {
		t.Fatalf("Accept: %v", err)
	}
}
