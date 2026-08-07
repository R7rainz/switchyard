package execution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

// collector records what the engine announced.
type collector struct {
	mu     sync.Mutex
	events []Event
}

func (c *collector) Publish(_ string, event any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event.(Event))
}

func (c *collector) all() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.events...)
}

// terminal returns the run's final event, or false if it never announced one.
func (c *collector) terminal() (Event, bool) {
	for _, event := range c.all() {
		if event.Type == EventExecution && event.Status.Done() {
			return event, true
		}
	}
	return Event{}, false
}

// failingStore is a store whose writes do not land.
type failingStore struct {
	*MemoryStore
	failFinish bool
	failNode   bool
}

var errStoreDown = errors.New("store: unavailable")

func (f *failingStore) Finish(ctx context.Context, id string, status Status, message string, at time.Time) error {
	if f.failFinish {
		return errStoreDown
	}
	return f.MemoryStore.Finish(ctx, id, status, message, at)
}

func (f *failingStore) finishIfRunning(ctx context.Context, id string, status Status, message string, at time.Time) (bool, error) {
	if f.failFinish {
		return false, errStoreDown
	}
	return f.MemoryStore.finishIfRunning(ctx, id, status, message, at)
}

func (f *failingStore) SaveNodeRun(ctx context.Context, run NodeRun) error {
	if f.failNode {
		return errStoreDown
	}
	return f.MemoryStore.SaveNodeRun(ctx, run)
}

func eventHarness(t *testing.T, graph workflow.Graph, store Store) (*Service, *collector) {
	t.Helper()

	events := &collector{}
	runners := Registry{"trigger.manual": RunnerFunc(runTrigger)}
	return NewService(store, stubWorkflows{graph: graph}, runners, Options{Events: events}), events
}

func triggerOnly() workflow.Graph {
	return workflow.Graph{Nodes: []workflow.Node{node("t", "trigger.manual", "")}}
}

// A finished run must announce it, so a watcher stops showing RUNNING.
func TestFinishIsAnnounced(t *testing.T) {
	store := NewMemoryStore()
	svc, events := eventHarness(t, triggerOnly(), store)

	run, err := svc.Start(t.Context(), "ws", "wf", "alice", TriggerManual, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	await(t, store, "ws", run.ID)

	terminal, ok := waitForTerminal(events)
	if !ok {
		t.Fatalf("no terminal event; got %+v", events.all())
	}
	if terminal.Status != StatusSucceeded {
		t.Fatalf("terminal event = %s %q", terminal.Status, terminal.Error)
	}
}

// A write that did not land must not be announced as though it had.
//
// Announcing a terminal status the database never took leaves the watcher
// showing SUCCEEDED while a refresh shows RUNNING, and later FAILED once
// Reclaim catches the row. Saying nothing is the honest outcome: the client
// keeps showing what the database agrees with.
func TestAFailedWriteIsNotAnnounced(t *testing.T) {
	store := &failingStore{MemoryStore: NewMemoryStore(), failFinish: true}
	svc, events := eventHarness(t, triggerOnly(), store)

	run, err := svc.Start(t.Context(), "ws", "wf", "alice", TriggerManual, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The row never reaches a terminal status, so there is nothing to await.
	time.Sleep(200 * time.Millisecond)

	if terminal, ok := events.terminal(); ok {
		t.Fatalf("announced %s for a write that failed: %+v", terminal.Status, terminal)
	}
	_ = run
}

func TestAFailedNodeWriteIsNotAnnounced(t *testing.T) {
	store := &failingStore{MemoryStore: NewMemoryStore(), failNode: true}
	svc, events := eventHarness(t, triggerOnly(), store)

	run, err := svc.Start(t.Context(), "ws", "wf", "alice", TriggerManual, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	await(t, store.MemoryStore, "ws", run.ID)

	for _, event := range events.all() {
		if event.Type == EventNode {
			t.Fatalf("announced a node write that failed: %+v", event)
		}
	}
}

// Cancelling a run this process is not running still finishes the row, so it
// still has to say so. Otherwise the watcher sits on RUNNING for a run that is
// cancelled in the database — the one outcome the engine must never have.
func TestCancellingARunNobodyIsRunningIsAnnounced(t *testing.T) {
	store := NewMemoryStore()
	svc, events := eventHarness(t, triggerOnly(), store)

	// A row that is running with nothing behind it, which is what a process
	// that died mid-run leaves.
	orphan := Execution{
		ID: "orphan", WorkspaceID: "ws", WorkflowID: "wf",
		Graph: triggerOnly(), Status: StatusRunning, Trigger: TriggerManual,
	}
	if err := store.Create(t.Context(), orphan); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Cancel(t.Context(), "ws", "orphan"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	terminal, ok := events.terminal()
	if !ok {
		t.Fatalf("cancelling announced nothing; got %+v", events.all())
	}
	if terminal.Status != StatusCancelled {
		t.Fatalf("terminal event = %s, want CANCELLED", terminal.Status)
	}
}

// A nil Publisher is the ordinary state of a test and of a process nothing has
// connected to.
func TestNoPublisherIsFine(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, stubWorkflows{graph: triggerOnly()},
		Registry{"trigger.manual": RunnerFunc(runTrigger)}, Options{})

	run, err := svc.Start(t.Context(), "ws", "wf", "alice", TriggerManual, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := await(t, store, "ws", run.ID); got.Status != StatusSucceeded {
		t.Fatalf("status = %s", got.Status)
	}
}

func waitForTerminal(events *collector) (Event, bool) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if event, ok := events.terminal(); ok {
			return event, true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return Event{}, false
}
