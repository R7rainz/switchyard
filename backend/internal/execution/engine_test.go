package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

const (
	wsA  = "ws-a"
	wsB  = "ws-b"
	user = "user-1"
)

// stubWorkflows serves one workflow, standing in for the workflow package.
type stubWorkflows struct {
	graph workflow.Graph
	err   error
}

func (s stubWorkflows) Get(_ context.Context, workspaceID, id string) (workflow.Workflow, error) {
	if s.err != nil {
		return workflow.Workflow{}, s.err
	}
	return workflow.Workflow{ID: id, WorkspaceID: workspaceID, Name: "test", Graph: s.graph}, nil
}

func node(id string, nodeType workflow.NodeType, data string) workflow.Node {
	n := workflow.Node{ID: id, Type: nodeType}
	if data != "" {
		n.Data = json.RawMessage(data)
	}
	return n
}

func edge(id, source, target string) workflow.Edge {
	return workflow.Edge{ID: id, Source: source, Target: target}
}

func branchEdge(id, source, target, handle string) workflow.Edge {
	return workflow.Edge{ID: id, Source: source, Target: target, SourceHandle: handle}
}

// recorder is a runner that notes the order it was called in and returns
// whatever it was told to.
type recorder struct {
	mu     sync.Mutex
	called []string
	out    map[string]string
	fail   map[string]error
	block  chan struct{}
}

func newRecorder() *recorder {
	return &recorder{out: map[string]string{}, fail: map[string]error{}}
}

func (r *recorder) Run(ctx context.Context, in Input) (Result, error) {
	r.mu.Lock()
	r.called = append(r.called, in.Node.ID)
	blocked := r.block
	failure := r.fail[in.Node.ID]
	output := r.out[in.Node.ID]
	r.mu.Unlock()

	if blocked != nil {
		select {
		case <-blocked:
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	if failure != nil {
		return Result{}, failure
	}
	if output == "" {
		output = `{"ok":true}`
	}
	return Result{Output: json.RawMessage(output)}, nil
}

func (r *recorder) order() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.called...)
}

// harness wires a service over memory with one recording runner for every node
// type the tests use.
func harness(t *testing.T, graph workflow.Graph) (*Service, *MemoryStore, *recorder) {
	t.Helper()

	rec := newRecorder()
	runners := Registry{}
	for _, nodeType := range []string{
		"trigger.manual", "http.request", "slack.message", "ai.prompt", "variable.set", "github.issue",
	} {
		runners.Register(nodeType, rec)
	}
	runners.Register("logic.condition", RunnerFunc(runCondition))

	store := NewMemoryStore()
	svc := NewService(store, stubWorkflows{graph: graph}, runners, Options{})
	return svc, store, rec
}

// await polls until the run reaches a terminal status. The engine is
// asynchronous by design, so every test has to wait for it somewhere.
func await(t *testing.T, store *MemoryStore, workspaceID, id string) Execution {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, err := store.Get(context.Background(), workspaceID, id)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if run.Status.Done() {
			return run
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("execution %s never finished", id)
	return Execution{}
}

func TestRunsNodesInDependencyOrder(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{
			node("t", "trigger.manual", ""),
			node("a", "http.request", ""),
			node("b", "slack.message", ""),
			node("c", "variable.set", ""),
		},
		Edges: []workflow.Edge{
			edge("e1", "t", "a"),
			edge("e2", "a", "b"),
			edge("e3", "b", "c"),
		},
	}
	svc, store, rec := harness(t, graph)

	run, err := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	finished := await(t, store, wsA, run.ID)

	if finished.Status != StatusSucceeded {
		t.Fatalf("status = %s, error = %q", finished.Status, finished.Error)
	}
	if got := strings.Join(rec.order(), ","); got != "t,a,b,c" {
		t.Fatalf("ran %q, want t,a,b,c", got)
	}
}

// The snapshot is the reason there is no version table, so it gets its own test.
func TestExecutionKeepsTheGraphItRan(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{node("t", "trigger.manual", ""), node("a", "http.request", "")},
		Edges: []workflow.Edge{edge("e1", "t", "a")},
	}
	svc, store, _ := harness(t, graph)

	run, err := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	await(t, store, wsA, run.ID)

	// The workflow changes completely after the run.
	svc.workflows = stubWorkflows{graph: workflow.Graph{
		Nodes: []workflow.Node{node("t2", "trigger.webhook", "")},
	}}

	stored, _, err := svc.Get(context.Background(), wsA, run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(stored.Graph.Nodes) != 2 || stored.Graph.Nodes[0].ID != "t" {
		t.Fatalf("the execution followed the workflow instead of its snapshot: %+v", stored.Graph)
	}
}

// A condition sends the run one way, and the other branch is recorded SKIPPED
// rather than left looking pending.
func TestBranchingSkipsTheUntakenPath(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{
			node("t", "trigger.manual", ""),
			node("cond", "logic.condition", `{"value":"true"}`),
			node("yes", "slack.message", ""),
			node("no", "github.issue", ""),
			node("after-no", "variable.set", ""),
		},
		Edges: []workflow.Edge{
			edge("e1", "t", "cond"),
			branchEdge("e2", "cond", "yes", "true"),
			branchEdge("e3", "cond", "no", "false"),
			edge("e4", "no", "after-no"),
		},
	}
	svc, store, rec := harness(t, graph)

	run, err := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	finished := await(t, store, wsA, run.ID)
	if finished.Status != StatusSucceeded {
		t.Fatalf("status = %s, error = %q", finished.Status, finished.Error)
	}

	if got := strings.Join(rec.order(), ","); got != "t,yes" {
		t.Fatalf("ran %q, want only the true branch", got)
	}

	statuses := map[string]Status{}
	rows, _ := store.NodeRuns(context.Background(), run.ID)
	for _, row := range rows {
		statuses[row.NodeID] = row.Status
	}
	if statuses["yes"] != StatusSucceeded {
		t.Fatalf("yes = %s", statuses["yes"])
	}
	// Both the untaken node and everything after it are skipped, not pending.
	for _, id := range []string{"no", "after-no"} {
		if statuses[id] != StatusSkipped {
			t.Fatalf("%s = %s, want SKIPPED", id, statuses[id])
		}
	}
}

func TestFailingNodeFailsTheRun(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{
			node("t", "trigger.manual", ""),
			node("a", "http.request", ""),
			node("b", "slack.message", ""),
		},
		Edges: []workflow.Edge{edge("e1", "t", "a"), edge("e2", "a", "b")},
	}
	svc, store, rec := harness(t, graph)
	rec.fail["a"] = errors.New("the api said no")

	run, _ := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	finished := await(t, store, wsA, run.ID)

	if finished.Status != StatusFailed {
		t.Fatalf("status = %s, want FAILED", finished.Status)
	}
	// The message names the node, because "which node failed" is the first
	// thing anyone looking at a failed run wants.
	if !strings.Contains(finished.Error, `node "a"`) || !strings.Contains(finished.Error, "the api said no") {
		t.Fatalf("error = %q", finished.Error)
	}
	if got := strings.Join(rec.order(), ","); got != "t,a" {
		t.Fatalf("ran %q, want the run to stop at a", got)
	}
}

// A runner that fails may still have produced the explanation. Dropping it is
// the difference between "node a failed" and "node a failed because the API
// said the token was expired", and the second is the whole point of the
// execution viewer.
func TestFailedNodeKeepsItsOutput(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{node("t", "trigger.manual", ""), node("a", "http.request", "")},
		Edges: []workflow.Edge{edge("e1", "t", "a")},
	}
	svc, store, _ := harness(t, graph)

	svc.runners.Register("http.request", RunnerFunc(func(context.Context, Input) (Result, error) {
		return Result{Output: json.RawMessage(`{"status":401,"body":"token expired"}`)},
			errors.New("http 401")
	}))

	run, _ := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	await(t, store, wsA, run.ID)

	rows, _ := store.NodeRuns(context.Background(), run.ID)
	for _, row := range rows {
		if row.NodeID != "a" {
			continue
		}
		if row.Status != StatusFailed {
			t.Fatalf("status = %s", row.Status)
		}
		if !strings.Contains(string(row.Output), "token expired") {
			t.Fatalf("output = %q, want the runner's explanation kept", row.Output)
		}
		return
	}
	t.Fatal("no row for node a")
}

// Output flows into the next node's data, which is what makes the engine more
// than a sequencer.
func TestOutputFlowsIntoTheNextNode(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{
			node("t", "trigger.manual", ""),
			node("fetch", "http.request", ""),
			node("post", "slack.message", `{"text":"got {{ .nodes.fetch.body.name }} from {{ .trigger.repo }}"}`),
		},
		Edges: []workflow.Edge{edge("e1", "t", "fetch"), edge("e2", "fetch", "post")},
	}
	svc, store, rec := harness(t, graph)
	rec.out["fetch"] = `{"body":{"name":"switchyard"}}`

	var seen json.RawMessage
	svc.runners.Register("slack.message", RunnerFunc(func(_ context.Context, in Input) (Result, error) {
		seen = in.Data
		return Result{Output: json.RawMessage(`{}`)}, nil
	}))

	run, _ := svc.Start(context.Background(), wsA, "wf-1", user, "", json.RawMessage(`{"repo":"main"}`))
	finished := await(t, store, wsA, run.ID)
	if finished.Status != StatusSucceeded {
		t.Fatalf("status = %s, error = %q", finished.Status, finished.Error)
	}

	want := `{"text":"got switchyard from main"}`
	if string(seen) != want {
		t.Fatalf("interpolated data = %s, want %s", seen, want)
	}
}

// A reference that does not resolve fails loudly. The alternative is a workflow
// that posts an empty string where an issue number should be.
func TestMissingReferenceFailsTheRun(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{
			node("t", "trigger.manual", ""),
			node("post", "slack.message", `{"text":"{{ .nodes.nonexistent.field }}"}`),
		},
		Edges: []workflow.Edge{edge("e1", "t", "post")},
	}
	svc, store, _ := harness(t, graph)

	run, _ := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	finished := await(t, store, wsA, run.ID)

	if finished.Status != StatusFailed {
		t.Fatalf("status = %s, want FAILED", finished.Status)
	}
}

func TestUnregisteredNodeTypeFailsTheRun(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{
			node("t", "trigger.manual", ""),
			node("a", "discord.message", ""), // a category Runnable allows, with no runner
		},
		Edges: []workflow.Edge{edge("e1", "t", "a")},
	}
	svc, store, _ := harness(t, graph)

	run, _ := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	finished := await(t, store, wsA, run.ID)

	if finished.Status != StatusFailed {
		t.Fatalf("status = %s, want FAILED", finished.Status)
	}
	if !strings.Contains(finished.Error, "discord.message") {
		t.Fatalf("error = %q, want it to name the node type", finished.Error)
	}
}

// A draft saves; starting it is where Runnable applies.
func TestStartRefusesAGraphThatCannotRun(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{node("floating", "http.request", "")},
	}
	svc, _, _ := harness(t, graph)

	_, err := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	if !errors.Is(err, workflow.ErrNotRunnable) {
		t.Fatalf("got %v, want ErrNotRunnable", err)
	}
}

func TestCancelStopsARunningExecution(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{node("t", "trigger.manual", ""), node("slow", "http.request", "")},
		Edges: []workflow.Edge{edge("e1", "t", "slow")},
	}
	svc, store, rec := harness(t, graph)
	rec.block = make(chan struct{})
	defer close(rec.block)

	run, err := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait until the slow node is actually in the runner, so the cancel has
	// something to interrupt rather than racing the start.
	deadline := time.Now().Add(2 * time.Second)
	for len(rec.order()) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if err := svc.Cancel(context.Background(), wsA, run.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	finished := await(t, store, wsA, run.ID)
	if finished.Status != StatusCancelled {
		t.Fatalf("status = %s, want CANCELLED", finished.Status)
	}

	// Cancelling something already finished is a 409, not a second cancel.
	if err := svc.Cancel(context.Background(), wsA, run.ID); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("second cancel: got %v, want ErrNotRunning", err)
	}
}

// A runner is somebody else's code. An unrecovered panic in a goroutine takes
// the whole process with it — chi's Recoverer only wraps handlers, never what
// they start — so the engine has to catch its own.
func TestPanickingRunnerFailsOnlyItsRun(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{node("t", "trigger.manual", ""), node("boom", "http.request", "")},
		Edges: []workflow.Edge{edge("e1", "t", "boom")},
	}
	svc, store, _ := harness(t, graph)
	svc.runners.Register("http.request", RunnerFunc(func(context.Context, Input) (Result, error) {
		panic("a badly written node")
	}))

	run, err := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	finished := await(t, store, wsA, run.ID)

	if finished.Status != StatusFailed {
		t.Fatalf("status = %s, want FAILED", finished.Status)
	}
	if !strings.Contains(finished.Error, "panicked") {
		t.Fatalf("error = %q, want it to say the engine panicked", finished.Error)
	}

	// And the process is still here to run the next one.
	second, err := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	if err != nil {
		t.Fatalf("the service did not survive: %v", err)
	}
	await(t, store, wsA, second.ID)
}

// A cancel must be honoured between nodes, not only inside them. Runners that
// return immediately never look at their context, so if the engine did not
// check, a graph of fast nodes would run to completion after being cancelled.
func TestCancelIsHonouredBetweenFastNodes(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{
			node("t", "trigger.manual", ""),
			node("a", "http.request", ""),
			node("b", "slack.message", ""),
		},
		Edges: []workflow.Edge{edge("e1", "t", "a"), edge("e2", "a", "b")},
	}
	svc, store, rec := harness(t, graph)

	// Every runner ignores its context entirely, which is the realistic case
	// for anything that finishes in microseconds.
	stop := make(chan struct{})
	svc.runners.Register("http.request", RunnerFunc(func(context.Context, Input) (Result, error) {
		<-stop // hold here only so the cancel has a moment to land
		return Result{Output: json.RawMessage(`{}`)}, nil
	}))

	run, _ := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	for len(rec.order()) < 1 {
		time.Sleep(time.Millisecond)
	}

	if err := svc.Cancel(context.Background(), wsA, run.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	close(stop)

	finished := await(t, store, wsA, run.ID)
	if finished.Status != StatusCancelled {
		t.Fatalf("status = %s, want CANCELLED", finished.Status)
	}
	// Node b comes after the cancel and must never have been reached.
	rows, _ := store.NodeRuns(context.Background(), run.ID)
	for _, row := range rows {
		if row.NodeID == "b" && row.Status != StatusSkipped {
			t.Fatalf("node b ran after the cancel: %s", row.Status)
		}
	}
}

// A run that outlives its limit failed; it was not cancelled. Saying otherwise
// sends somebody looking for the person who stopped it.
func TestRunTimeoutIsFailedNotCancelled(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{node("t", "trigger.manual", ""), node("slow", "http.request", "")},
		Edges: []workflow.Edge{edge("e1", "t", "slow")},
	}
	svc, store, _ := harness(t, graph)
	svc.runTimeout = 50 * time.Millisecond
	svc.runners.Register("http.request", RunnerFunc(func(ctx context.Context, _ Input) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}))

	run, _ := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	finished := await(t, store, wsA, run.ID)

	if finished.Status != StatusFailed {
		t.Fatalf("status = %s, want FAILED", finished.Status)
	}
	if !strings.Contains(finished.Error, "exceeded") {
		t.Fatalf("error = %q, want it to name the limit", finished.Error)
	}
}

// A run in one workspace is invisible from another, the same rule as everywhere
// else and for the same reason.
func TestExecutionIsWorkspaceScoped(t *testing.T) {
	graph := workflow.Graph{Nodes: []workflow.Node{node("t", "trigger.manual", "")}}
	svc, store, _ := harness(t, graph)

	run, _ := svc.Start(context.Background(), wsA, "wf-1", user, "", nil)
	await(t, store, wsA, run.ID)

	if _, _, err := svc.Get(context.Background(), wsB, run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get from another workspace: got %v, want ErrNotFound", err)
	}
	if err := svc.Cancel(context.Background(), wsB, run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Cancel from another workspace: got %v, want ErrNotFound", err)
	}
}

// A run left behind by a dead process must not sit at RUNNING forever, because
// that reads as "the engine is stuck" rather than "this never finished".
func TestReclaimFailsInterruptedRuns(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, stubWorkflows{}, Registry{}, Options{})

	for i, status := range []Status{StatusPending, StatusRunning, StatusSucceeded} {
		err := store.Create(context.Background(), Execution{
			ID: fmt.Sprintf("run-%d", i), WorkspaceID: wsA, Status: status,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	reclaimed, err := svc.Reclaim(context.Background())
	if err != nil {
		t.Fatalf("Reclaim: %v", err)
	}
	if reclaimed != 2 {
		t.Fatalf("reclaimed %d, want 2", reclaimed)
	}

	done, _ := store.Get(context.Background(), wsA, "run-2")
	if done.Status != StatusSucceeded {
		t.Fatalf("a finished run was reclaimed: %s", done.Status)
	}
}
