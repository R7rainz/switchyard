package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/ai"
	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
	"github.com/R7rainz/switchyard/backend/internal/workspace"
)

// modelReturns is a provider that always answers with the same text, so these
// tests exercise the route rather than a model.
type modelReturns string

func (m modelReturns) Complete(context.Context, string, ai.Request) (ai.Response, error) {
	return ai.Response{Text: string(m), Model: "stub"}, nil
}

// keyStored decides whether the workspace has an API key.
type keyStored bool

func (k keyStored) Get(context.Context, string, string, string) (credential.Secret, error) {
	if !k {
		return nil, credential.ErrNotFound
	}
	return credential.Secret("sk-test"), nil
}

const generatedGraph = `{"name":"nightly build","description":"runs the build","graph":{
	"nodes":[
		{"id":"t","type":"trigger.manual","position":{"x":0,"y":0},"data":{"label":"Start"}},
		{"id":"a","type":"http.request","position":{"x":280,"y":0},"data":{"label":"Build","url":"https://ci.example.com"}}
	],
	"edges":[{"id":"e1","source":"t","target":"a"}]}}`

func aiRouter(t *testing.T, reply string, hasKey bool) (http.Handler, *workspace.Service, *workflow.Service) {
	t.Helper()

	workspaces := workspace.NewService(workspace.NewMemoryStore())
	workflows := workflow.NewService(workflow.NewMemoryStore())
	router := NewRouter(Deps{
		Verifier:   tokenNamesTheCaller{},
		Logger:     testLogger(),
		Workspaces: workspaces,
		Workflows:  workflows,
		AI:         ai.NewService(modelReturns(reply), keyStored(hasKey)),
		AppURL:     testAppURL,
	})
	return router, workspaces, workflows
}

// Generating hands back a graph and stores nothing. The user reviews it on the
// canvas and saves it themselves — a workflow appearing in the list before
// anybody has read it is the behaviour this asserts against.
func TestGenerateReturnsAGraphAndSavesNothing(t *testing.T) {
	router, _, workflows := aiRouter(t, generatedGraph, true)
	ws := firstWorkspace(t, router, "alice")

	status, body := call(t, router, http.MethodPost, "/api/workspaces/"+ws+"/workflows/generate", "alice",
		`{"prompt":"run the build every night"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, body %v", status, body)
	}
	if field(t, body, "name") != "nightly build" {
		t.Fatalf("name = %v", body["name"])
	}

	graph, _ := body["graph"].(map[string]any)
	if nodes, _ := graph["nodes"].([]any); len(nodes) != 2 {
		t.Fatalf("graph did not reach the caller: %v", body["graph"])
	}

	stored, err := workflows.List(t.Context(), ws)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("generating stored %d workflows, want 0", len(stored))
	}
}

// The generate route is not a static-versus-wildcard collision: "generate"
// must not be read as a workflow id.
func TestGenerateDoesNotCollideWithWorkflowRoutes(t *testing.T) {
	router, _, _ := aiRouter(t, generatedGraph, true)
	ws := firstWorkspace(t, router, "alice")

	status, _ := call(t, router, http.MethodPost, "/api/workspaces/"+ws+"/workflows", "alice",
		`{"name":"hand made","graph":`+smallGraph+`}`)
	if status != http.StatusCreated {
		t.Fatalf("creating by hand still has to work: status = %d", status)
	}
	status, _ = call(t, router, http.MethodPost, "/api/workspaces/"+ws+"/workflows/generate", "alice",
		`{"prompt":"x"}`)
	if status != http.StatusOK {
		t.Fatalf("generate: status = %d", status)
	}
}

// A workspace with no key gets a 400 naming the fix, not a 502 that reads as
// our fault.
func TestGenerateWithoutAnAPIKey(t *testing.T) {
	router, _, _ := aiRouter(t, generatedGraph, false)
	ws := firstWorkspace(t, router, "alice")

	status, body := call(t, router, http.MethodPost, "/api/workspaces/"+ws+"/workflows/generate", "alice",
		`{"prompt":"anything"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", status, body)
	}
	if message := field(t, body, "error"); !strings.Contains(message, "openrouter") {
		t.Fatalf("error %q does not say which key is missing", message)
	}
}

// A model answering with prose rather than a workflow is an upstream failure.
func TestGenerateWhenTheModelAnswersWithNonsense(t *testing.T) {
	router, _, _ := aiRouter(t, "I'd be happy to help with that!", true)
	ws := firstWorkspace(t, router, "alice")

	status, body := call(t, router, http.MethodPost, "/api/workspaces/"+ws+"/workflows/generate", "alice",
		`{"prompt":"anything"}`)
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %v", status, body)
	}
}

func TestGenerateNeedsAPrompt(t *testing.T) {
	router, _, _ := aiRouter(t, generatedGraph, true)
	ws := firstWorkspace(t, router, "alice")

	status, _ := call(t, router, http.MethodPost, "/api/workspaces/"+ws+"/workflows/generate", "alice",
		`{"prompt":"   "}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

// Generating spends the workspace's model budget, so it sits with writing
// rather than reading.
func TestGenerateNeedsWritePermission(t *testing.T) {
	router, workspaces, _ := aiRouter(t, generatedGraph, true)
	ws := firstWorkspace(t, router, "alice")
	joinAs(t, workspaces, ws, "viewer", auth.RoleViewer)

	status, _ := call(t, router, http.MethodPost, "/api/workspaces/"+ws+"/workflows/generate", "viewer",
		`{"prompt":"x"}`)
	if status != http.StatusForbidden {
		t.Fatalf("viewer: status = %d, want 403", status)
	}

	status, _ = call(t, router, http.MethodPost, "/api/workspaces/"+ws+"/workflows/generate", "stranger",
		`{"prompt":"x"}`)
	if status != http.StatusNotFound {
		t.Fatalf("stranger: status = %d, want 404", status)
	}
}
