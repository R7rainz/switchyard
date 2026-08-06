package execution

import (
	"context"
	"encoding/json"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

// How a run was started. These are the values the trigger column takes.
const (
	TriggerManual   = "manual"
	TriggerWebhook  = "webhook"
	TriggerSchedule = "schedule"
	TriggerRetry    = "retry"
)

// Input is everything a node is given.
type Input struct {
	// Node is the node as drawn, including its untouched Data.
	Node workflow.Node

	// Data is Node.Data with variables substituted, which is what a runner
	// should actually read. The raw form stays available for a runner that
	// wants the template rather than its result.
	Data json.RawMessage

	// Outputs is what every node so far returned, keyed by node id.
	Outputs map[string]json.RawMessage

	// Trigger is the payload the run started with.
	Trigger json.RawMessage

	// WorkspaceID is here so a runner can fetch a credential. It is the only
	// reason a runner needs to know it, and credentials are looked up rather
	// than passed in so a secret is never sitting in a struct the engine logs.
	WorkspaceID string
}

// Result is what a node produced.
type Result struct {
	// Output is handed to downstream nodes and stored on the node's row. It
	// must not contain a secret: it is returned over HTTP and will be
	// streamed to a browser.
	Output json.RawMessage

	// Branch names which output the run leaves by, and is how a condition node
	// picks a path. Empty follows every edge with no sourceHandle, which is the
	// normal single-output case.
	Branch string
}

// Runner executes one node type.
//
// It is small on purpose: everything a node can do to the run is in its Result,
// so a badly behaved runner can fail its own node and nothing else. Runners
// live in the package that owns the integration — github in internal/github,
// and so on — except the two below, which need nothing but the standard
// library and would be a package containing one function.
type Runner interface {
	Run(ctx context.Context, in Input) (Result, error)
}

// RunnerFunc adapts a function to Runner.
type RunnerFunc func(ctx context.Context, in Input) (Result, error)

func (f RunnerFunc) Run(ctx context.Context, in Input) (Result, error) { return f(ctx, in) }

// Registry maps a node type to the thing that runs it. A node type with no
// entry fails its run with a message naming the type, rather than being
// skipped: silently doing nothing is the one outcome a workflow engine must
// never have.
type Registry map[string]Runner

// Register adds a runner, replacing any already there.
func (r Registry) Register(nodeType string, runner Runner) { r[nodeType] = runner }

// Add merges another registry in, which is how an integration package hands
// over everything it provides in one call.
func (r Registry) Add(other Registry) {
	for nodeType, runner := range other {
		r[nodeType] = runner
	}
}
