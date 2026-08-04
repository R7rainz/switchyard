package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	coder "github.com/coder/websocket"

	"github.com/R7rainz/switchyard/backend/internal/auth"
	"github.com/R7rainz/switchyard/backend/internal/execution"
	"github.com/R7rainz/switchyard/backend/internal/websocket"
	"github.com/R7rainz/switchyard/backend/internal/workflow"
	"github.com/R7rainz/switchyard/backend/internal/workspace"
)

// runnableGraph is a two-node graph that runs to completion, so a stream has
// real events to carry rather than fabricated ones.
const runnableGraph = `{
	"nodes": [
		{"id": "t", "type": "trigger.manual", "position": {"x": 0, "y": 0}, "data": {"label": "Start"}},
		{"id": "a", "type": "logic.condition", "position": {"x": 280, "y": 0}, "data": {"label": "Check", "value": "true"}}
	],
	"edges": [{"id": "e1", "source": "t", "target": "a"}]
}`

// gatedGraph holds the run open on an HTTP node until the test lets it go.
//
// Without it these tests race the engine: two fast nodes finish before a socket
// can be opened, the events go to a topic nobody is watching yet, and the test
// fails for a reason that has nothing to do with streaming.
func gatedGraph(url string) string {
	return `{
		"nodes": [
			{"id": "t", "type": "trigger.manual", "position": {"x": 0, "y": 0}, "data": {"label": "Start"}},
			{"id": "a", "type": "http.request", "position": {"x": 280, "y": 0}, "data": {"label": "Wait", "url": "` + url + `"}}
		],
		"edges": [{"id": "e1", "source": "t", "target": "a"}]
	}`
}

// gate is an endpoint that does not answer until released.
func gate(t *testing.T) (url string, release func()) {
	t.Helper()

	released := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(released) }) }

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-released:
		case <-r.Context().Done():
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	// Registered after Close so it runs before it: cleanups are LIFO, and
	// closing a server with a handler still parked inside it hangs.
	t.Cleanup(server.Close)
	t.Cleanup(release)

	return server.URL, release
}

type streamHarness struct {
	url        string
	handler    http.Handler
	workspaces *workspace.Service
	hub        *websocket.Hub
}

// streamServer runs the production router on a real listener, with the same
// write timeout the API server uses, so the stream meets the middleware chain
// and the connection handling it will meet in production.
func streamServer(t *testing.T) streamHarness {
	t.Helper()

	workspaces := workspace.NewService(workspace.NewMemoryStore())
	workflows := workflow.NewService(workflow.NewMemoryStore())
	hub := websocket.NewHub(testAppURL)
	executions := execution.NewService(
		execution.NewMemoryStore(), workflows, execution.Builtin(nil),
		execution.Options{Events: hub})

	handler := NewRouter(Deps{
		Verifier:   tokenNamesTheCaller{},
		Logger:     testLogger(),
		Workspaces: workspaces,
		Workflows:  workflows,
		Executions: executions,
		Events:     hub,
		AppURL:     testAppURL,
	})

	server := httptest.NewUnstartedServer(handler)
	server.Config.WriteTimeout = 30 * time.Second
	server.Start()
	t.Cleanup(server.Close)

	return streamHarness{
		url:        "ws" + strings.TrimPrefix(server.URL, "http"),
		handler:    handler,
		workspaces: workspaces,
		hub:        hub,
	}
}

// watch opens the stream as userID, carrying the token the way a browser has
// to: the WebSocket constructor cannot set headers, so it rides in the
// subprotocol.
func watch(t *testing.T, h streamHarness, workspaceID, executionID, userID string) (*coder.Conn, *http.Response, error) {
	t.Helper()

	url := h.url + "/api/workspaces/" + workspaceID + "/executions/" + executionID + "/events"
	conn, response, err := coder.Dial(t.Context(), url, &coder.DialOptions{
		Subprotocols: []string{"bearer", userID},
	})
	if conn != nil {
		t.Cleanup(func() { _ = conn.CloseNow() })
	}
	return conn, response, err
}

func nextEvent(t *testing.T, conn *coder.Conn) execution.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, raw, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var event execution.Event
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("event is not an execution.Event: %s", raw)
	}
	return event
}

// A run's progress reaches a watcher: the run starting, each node, and the
// outcome. This is the explainability requirement arriving live rather than on
// a refresh.
func TestStreamCarriesARunsProgress(t *testing.T) {
	h := streamServer(t)
	ws := firstWorkspace(t, h.handler, "alice")
	base := "/api/workspaces/" + ws

	url, release := gate(t)
	_, created := call(t, h.handler, http.MethodPost, base+"/workflows", "alice",
		`{"name":"streamed","graph":`+gatedGraph(url)+`}`)
	workflowID := field(t, created, "id")

	status, started := call(t, h.handler, http.MethodPost,
		base+"/workflows/"+workflowID+"/executions", "alice", "")
	if status != http.StatusAccepted {
		t.Fatalf("start: status = %d, body %v", status, started)
	}
	runID := field(t, started, "id")

	// The run is parked on the HTTP node, so subscribing here cannot miss its
	// ending — which is the ordering a client has to follow too: connect, then
	// fetch. Fetching first leaves a window where the run finishes unobserved
	// and the page shows RUNNING forever.
	conn, _, err := watch(t, h, ws, runID, "alice")
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	release()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		event := nextEvent(t, conn)
		if event.ExecutionID != runID {
			t.Fatalf("event for another run: %+v", event)
		}
		if event.Type == execution.EventExecution && event.Status.Done() {
			if event.Status != execution.StatusSucceeded {
				t.Fatalf("run finished %s: %s", event.Status, event.Error)
			}
			return
		}
	}
	t.Fatal("the run never reported finishing")
}

// A node's own result arrives, not just the run's, or the viewer cannot show
// which step is where.
func TestStreamCarriesNodeResults(t *testing.T) {
	h := streamServer(t)
	ws := firstWorkspace(t, h.handler, "alice")
	base := "/api/workspaces/" + ws

	url, release := gate(t)
	_, created := call(t, h.handler, http.MethodPost, base+"/workflows", "alice",
		`{"name":"streamed","graph":`+gatedGraph(url)+`}`)
	workflowID := field(t, created, "id")

	_, started := call(t, h.handler, http.MethodPost,
		base+"/workflows/"+workflowID+"/executions", "alice", "")
	runID := field(t, started, "id")

	conn, _, err := watch(t, h, ws, runID, "alice")
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	release()

	seenNode := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		event := nextEvent(t, conn)
		if event.Type == execution.EventNode && event.NodeID != "" {
			seenNode = true
		}
		if event.Type == execution.EventExecution && event.Status.Done() {
			break
		}
	}
	if !seenNode {
		t.Fatal("no node event arrived; the viewer would have nothing to draw")
	}
}

// The stream sits behind the same auth as everything else. A browser cannot set
// headers, so the subprotocol is the only way in — and no credential at all is
// still a 401.
func TestStreamRequiresAToken(t *testing.T) {
	h := streamServer(t)
	ws := firstWorkspace(t, h.handler, "alice")

	url := h.url + "/api/workspaces/" + ws + "/executions/anything/events"
	conn, response, err := coder.Dial(t.Context(), url, nil)
	if conn != nil {
		_ = conn.CloseNow()
	}
	if err == nil {
		t.Fatal("an unauthenticated upgrade succeeded")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", response)
	}
}

// RequirePermission checks the workspace in the URL and knows nothing about the
// {executionID} beside it. Without the lookup in the handler, read access to
// one workspace would be read access to every run on the server.
func TestStreamWillNotWatchAnotherWorkspacesRun(t *testing.T) {
	h := streamServer(t)

	alice := firstWorkspace(t, h.handler, "alice")
	bob := firstWorkspace(t, h.handler, "bob")

	_, created := call(t, h.handler, http.MethodPost, "/api/workspaces/"+alice+"/workflows", "alice",
		`{"name":"alice's","graph":`+runnableGraph+`}`)
	_, started := call(t, h.handler, http.MethodPost,
		"/api/workspaces/"+alice+"/workflows/"+field(t, created, "id")+"/executions", "alice", "")
	runID := field(t, started, "id")

	// Bob owns his workspace, so this is not a permission failure. It is a
	// lookup that has to miss.
	conn, response, err := watch(t, h, bob, runID, "bob")
	if conn != nil {
		_ = conn.CloseNow()
	}
	if err == nil {
		t.Fatal("bob subscribed to alice's run")
	}
	if response == nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %v, want 404", response)
	}

	// And nobody is left attached to the topic.
	if watching := h.hub.Subscribers(execution.Topic(runID)); watching != 0 {
		t.Fatalf("%d subscribers after a refused upgrade", watching)
	}
}

// A viewer may watch what happened; making something happen takes a member.
func TestStreamIsOpenToViewers(t *testing.T) {
	h := streamServer(t)
	ws := firstWorkspace(t, h.handler, "alice")
	joinAs(t, h.workspaces, ws, "viewer", auth.RoleViewer)

	_, created := call(t, h.handler, http.MethodPost, "/api/workspaces/"+ws+"/workflows", "alice",
		`{"name":"streamed","graph":`+runnableGraph+`}`)
	_, started := call(t, h.handler, http.MethodPost,
		"/api/workspaces/"+ws+"/workflows/"+field(t, created, "id")+"/executions", "alice", "")

	conn, response, err := watch(t, h, ws, field(t, started, "id"), "viewer")
	if err != nil {
		t.Fatalf("a viewer could not watch: %v (%v)", err, response)
	}
	_ = conn.CloseNow()

	// A stranger gets 404, so run ids cannot be probed.
	conn, response, err = watch(t, h, ws, field(t, started, "id"), "stranger")
	if conn != nil {
		_ = conn.CloseNow()
	}
	if err == nil {
		t.Fatal("a stranger subscribed")
	}
	if response == nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("stranger status = %v, want 404", response)
	}
}
