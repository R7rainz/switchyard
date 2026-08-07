package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

// liveRuns holds the stop func for every execution running in this process.
type liveRuns struct {
	mu    sync.Mutex
	stops map[string]context.CancelFunc
}

func newLiveRuns() *liveRuns {
	return &liveRuns{stops: make(map[string]context.CancelFunc)}
}

func (l *liveRuns) add(id string, stop context.CancelFunc) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stops[id] = stop
}

func (l *liveRuns) remove(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.stops, id)
}

// cancel reports whether there was anything here to cancel.
func (l *liveRuns) cancel(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	stop, ok := l.stops[id]
	if ok {
		stop()
	}
	return ok
}

// errCancelled separates a stopped run from a node that failed on its own.
var errCancelled = errors.New("execution: cancelled")

// runStopped reports whether the run's context has ended, and distinguishes the
// two ways that happens.
//
// Both arrive as a dead context, and conflating them would tell somebody their
// run was cancelled when nobody cancelled it — it simply took too long, which
// is a completely different thing to go and fix.
func runStopped(ctx context.Context, limit time.Duration) (Status, string, bool) {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return StatusFailed, fmt.Sprintf("the run exceeded its %s limit", limit), true
	case ctx.Err() != nil:
		return StatusCancelled, "cancelled", true
	}
	return "", "", false
}

// launch starts the run in the background.
//
// context.WithoutCancel is the whole trick here. The request's context is
// cancelled the moment the response is written, so handing it to the goroutine
// would kill every execution the instant it was accepted. WithoutCancel drops
// that cancellation while keeping the values — the request's logger, so a run's
// lines can still be tied back to the call that started it — and the run then
// gets a deadline of its own.
func (s *Service) launch(parent context.Context, run Execution) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), s.runTimeout)
	s.live.add(run.ID, cancel)

	go func() {
		defer cancel()
		defer s.live.remove(run.ID)
		s.run(ctx, run)
	}()
}

// run walks the graph and records what happened. It never returns an error:
// there is nobody left to return one to, so every outcome is written to the
// execution row instead.
func (s *Service) run(ctx context.Context, run Execution) {
	logger := zerolog.Ctx(ctx).With().Str("execution_id", run.ID).Logger()

	// A runner is somebody else's code, and an unrecovered panic in a goroutine
	// takes the whole process with it — chi's Recoverer only wraps handlers,
	// never anything they start. One badly written node must fail one run.
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error().
				Interface("panic", recovered).
				Bytes("stack", debug.Stack()).
				Msg("execution panicked")
			s.finish(ctx, run.ID, StatusFailed, fmt.Sprintf("the engine panicked: %v", recovered))
		}
	}()

	if err := s.store.Start(ctx, run.ID, s.now()); err != nil {
		// Nothing can be recorded if the store is unreachable, and Reclaim will
		// catch the row on the next restart.
		logger.Error().Err(err).Msg("could not start execution")
		return
	}
	// Announced after the row moves, never before: a watcher must not see
	// RUNNING for a run the database does not agree has started.
	s.publish(Event{Type: EventExecution, ExecutionID: run.ID, Status: StatusRunning})

	status, message := s.walk(ctx, run)
	s.finish(ctx, run.ID, status, message)

	event := logger.Info()
	if status != StatusSucceeded {
		event = logger.Warn()
	}
	event.Str("status", string(status)).Str("reason", message).Msg("execution finished")
}

// finish writes the outcome on a context of its own.
//
// The run's context may already be cancelled or timed out — that is often
// exactly why we are here — and this is the one write that must not be skipped,
// or the row stays RUNNING until the next restart reclaims it.
func (s *Service) finish(ctx context.Context, id string, status Status, message string) {
	write, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	transitioned := true
	var err error
	if finisher, ok := s.store.(transitionFinisher); ok {
		transitioned, err = finisher.finishIfRunning(write, id, status, message, s.now())
	} else {
		err = s.store.Finish(write, id, status, message, s.now())
	}
	if err != nil {
		// Not announced, on purpose. Saying SUCCEEDED for a write that did not
		// land leaves the watcher showing one thing while a refresh shows
		// another, and later a third once Reclaim catches the row. Silence is
		// honest: the client keeps showing what the database agrees with.
		zerolog.Ctx(ctx).Error().Err(err).Str("execution_id", id).Msg("could not finish execution")
		return
	}
	if !transitioned {
		return
	}
	s.publish(Event{Type: EventExecution, ExecutionID: id, Status: status, Error: message})
}

// walk runs the nodes in dependency order and returns the run's outcome.
//
// Nodes run one at a time. Two independent branches could go in parallel, but
// sequential is the version that is obviously correct, and the execution viewer
// reads as a list either way — parallelism is an optimisation to make when a
// real workflow is visibly waiting on it.
func (s *Service) walk(ctx context.Context, run Execution) (Status, string) {
	graph := run.Graph
	nodes := make(map[string]workflow.Node, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
	}

	order, err := topological(graph)
	if err != nil {
		// Runnable already proved this graph is acyclic, so reaching here means
		// the snapshot was tampered with in storage.
		return StatusFailed, err.Error()
	}

	// outputs feeds the next node's data interpolation, keyed by node id.
	outputs := make(map[string]json.RawMessage, len(graph.Nodes))

	// active marks the nodes the run actually reaches. The trigger starts
	// active; everything else earns it by having an incoming edge taken.
	active := map[string]bool{triggerOf(graph): true}

	for _, id := range order {
		// Checked between nodes, not only inside them. A runner that finishes
		// quickly never notices its context, so without this a cancel would be
		// ignored by any graph of fast nodes — the engine has to be the one
		// that stops, rather than trusting every runner to.
		if status, message, stopped := runStopped(ctx, s.runTimeout); stopped {
			return status, message
		}

		node := nodes[id]

		if !active[id] {
			// Downstream of a branch that was not taken. Recorded rather than
			// left out, so the viewer can show it greyed instead of pending.
			s.record(ctx, NodeRun{ExecutionID: run.ID, NodeID: id, Status: StatusSkipped})
			continue
		}

		result, err := s.runNode(ctx, run, node, outputs)
		switch {
		case errors.Is(err, errCancelled):
			status, message, _ := runStopped(ctx, s.runTimeout)
			return status, message
		case err != nil:
			// One failed node fails the run. Continue-on-error belongs to the
			// node's own configuration, and no node has asked for it yet.
			return StatusFailed, fmt.Sprintf("node %q: %v", id, err)
		}

		outputs[id] = result.Output
		for _, edge := range graph.Edges {
			if edge.Source == id && takes(edge, result.Branch) {
				active[edge.Target] = true
			}
		}
	}

	return StatusSucceeded, ""
}

// takes reports whether an edge leaving a node is followed.
//
// An edge with no sourceHandle is the default path and is always followed. One
// with a handle is followed only when the node named that branch, which is how
// a condition node sends the run down "true" or "false".
func takes(edge workflow.Edge, branch string) bool {
	return edge.SourceHandle == "" || edge.SourceHandle == branch
}

// runNode executes one node and records the row for it, before and after, so a
// viewer polling mid-run sees RUNNING rather than nothing.
func (s *Service) runNode(ctx context.Context, run Execution, node workflow.Node, outputs map[string]json.RawMessage) (Result, error) {
	started := s.now()
	s.record(ctx, NodeRun{
		ExecutionID: run.ID,
		NodeID:      node.ID,
		Status:      StatusRunning,
		StartedAt:   started,
	})

	result, err := s.dispatch(ctx, run, node, outputs)

	row := NodeRun{
		ExecutionID: run.ID,
		NodeID:      node.ID,
		Status:      StatusSucceeded,
		Output:      result.Output,
		StartedAt:   started,
		FinishedAt:  s.now(),
	}
	if err != nil {
		row.Status = StatusFailed
		row.Error = err.Error()
		if errors.Is(err, errCancelled) {
			row.Status = StatusCancelled
		}
		// Whatever the runner managed to produce is kept. An HTTP node that
		// failed on a 500 still has the response body, and that body is
		// usually the whole explanation.
	}
	s.record(ctx, row)

	return result, err
}

// dispatch resolves the node's runner, interpolates its data, and calls it
// under a per-node timeout.
func (s *Service) dispatch(ctx context.Context, run Execution, node workflow.Node, outputs map[string]json.RawMessage) (Result, error) {
	runner, ok := s.runners[string(node.Type)]
	if !ok {
		// Runnable only checks the category, on purpose — the frontend may ship
		// a node type before the backend has a runner for it. This is where
		// that bill comes due, and it is a failed run rather than a failed save.
		return Result{}, fmt.Errorf("no runner registered for node type %q", node.Type)
	}

	data, err := interpolate(node.Data, outputs, run.Input)
	if err != nil {
		return Result{}, err
	}

	nodeCtx, cancel := context.WithTimeout(ctx, s.nodeTimeout)
	defer cancel()

	result, err := runner.Run(nodeCtx, Input{
		Node:        node,
		Data:        data,
		Outputs:     outputs,
		Trigger:     run.Input,
		WorkspaceID: run.WorkspaceID,
	})
	if err != nil {
		// The result is returned alongside the error, never dropped. A runner
		// that failed may still have produced the explanation — an HTTP node
		// rejected with a 401 holds the response body saying why — and that is
		// exactly what somebody reading a failed run needs.
		//
		// A cancelled parent means the user stopped the run; a node that simply
		// ran long is the node's own failure. Distinguishing them is what makes
		// the execution view honest about whose fault it was.
		if ctx.Err() != nil {
			return result, errCancelled
		}
		return result, err
	}
	return result, nil
}

// record writes a node row, ignoring the error. Failing to log must not fail
// the run: the node already did its work, and losing the record of it is worse
// repeated as a failure than left as a gap.
func (s *Service) record(ctx context.Context, row NodeRun) {
	write, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.store.SaveNodeRun(write, row); err != nil {
		// The node already did its work, so a lost record is a gap rather than
		// a failure and the run carries on. It is not announced either: an
		// event for a row that is not there would show the viewer a node the
		// execution's own REST response does not have.
		zerolog.Ctx(ctx).Warn().Err(err).
			Str("execution_id", row.ExecutionID).Str("node_id", row.NodeID).
			Msg("could not record node run")
		return
	}

	// Every node transition passes through here — RUNNING, then its outcome,
	// and SKIPPED for a branch not taken — so this one call covers the lot.
	s.publish(Event{
		Type:        EventNode,
		ExecutionID: row.ExecutionID,
		NodeID:      row.NodeID,
		Status:      row.Status,
		Output:      row.Output,
		Error:       row.Error,
	})
}

// triggerOf returns the graph's trigger. Runnable has already proved there is
// exactly one.
func triggerOf(graph workflow.Graph) string {
	for _, node := range graph.Nodes {
		if node.Type.IsTrigger() {
			return node.ID
		}
	}
	return ""
}

// topological orders the nodes so every node comes after everything feeding it.
// Kahn's algorithm again, the same one Runnable uses to prove there is an order
// at all; this is the order it proved exists.
func topological(graph workflow.Graph) ([]string, error) {
	indegree := make(map[string]int, len(graph.Nodes))
	successors := make(map[string][]string, len(graph.Nodes))
	for _, node := range graph.Nodes {
		indegree[node.ID] = 0
	}
	for _, edge := range graph.Edges {
		indegree[edge.Target]++
		successors[edge.Source] = append(successors[edge.Source], edge.Target)
	}

	// Seeded in graph order rather than map order, so a graph with several
	// ready nodes runs the same way every time. Deterministic execution is the
	// promise; a map range would quietly break it.
	var queue []string
	for _, node := range graph.Nodes {
		if indegree[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}

	order := make([]string, 0, len(graph.Nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)

		for _, next := range successors[id] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(order) != len(graph.Nodes) {
		return nil, errors.New("execution: the stored graph has a cycle")
	}
	return order, nil
}
