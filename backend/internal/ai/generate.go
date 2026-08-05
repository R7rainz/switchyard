package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

// ErrBadGraph means the model answered with something that is not a workflow.
// It is separated from ErrProvider because the call succeeded — the content is
// the problem, and that is worth a different status and a different fix.
var ErrBadGraph = errors.New("ai: the model did not return a usable workflow")

// Generated is a proposal. Nothing is stored: the user edits it on the canvas
// and saves it themselves, because AI assists and never owns.
type Generated struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Graph       workflow.Graph `json:"graph"`
}

// generateTokens bounds one proposal. A workflow graph is small; a model
// rambling past this is not producing one.
const generateTokens = 4000

// generateTimeout bounds both attempts together, under the API server's 30s
// write timeout.
const generateTimeout = 25 * time.Second

// systemPrompt describes the graph the frontend already speaks. It lists only
// node types that have a runner registered — advertising one the engine cannot
// execute buys a workflow that saves and then fails on its first run.
//
// Update this when a runner package lands.
const systemPrompt = `You design automation workflows for software teams and return them as JSON.

Answer with a single JSON object and nothing else:

{"name": "...", "description": "...", "graph": {"nodes": [...], "edges": [...]}}

A node is {"id", "type", "position": {"x", "y"}, "data": {...}}.
An edge is {"id", "source", "target"} and optionally "sourceHandle".

Rules:
- Exactly one trigger node. Nothing may point at it.
- Every node must be reachable from the trigger. No cycles.
- Node ids are short and unique, like "trigger", "fetch", "notify".
- Lay nodes out left to right, 280 apart in x, branches separated in y.
- Every node's data has a short "label" for the canvas.

Node types:
- trigger.manual    {"label"}                     started by a person. Prefer this.
- trigger.webhook   {"label"}                     started by an inbound call.
- trigger.schedule  {"label", "cron"}             started on a schedule.
- trigger.github.pull_request {"label", "action"}  starts on a signed GitHub pull_request delivery.
- logic.condition   {"label", "value"}            value renders to true/false. It must
                    have two outgoing edges, with sourceHandle "true" and "false".
- variable.set      {"label", "values"}           values is an object of name/value pairs.
                    Later nodes read them as {{ .nodes.<id>.<name> }}. Use it to name a
                    value several nodes need, instead of repeating the expression.
- http.request      {"label", "method", "url", "headers", "body"}
- ai.prompt         {"label", "prompt", "system"} asks a model and returns {"text"}.
- github.pull_request {"label", "owner", "repo", "number"} returns PR title, body, author, branches, and URL.
- slack.message     {"label", "text"}              posts text to the workspace Slack webhook.

Refer to earlier nodes with Go template syntax inside a string:
  {{ .nodes.<id>.<field> }} for a node's output, {{ .trigger.<field> }} for the payload.
An http.request node outputs {"status", "body"}, so a later node reads
{{ .nodes.fetch.body.name }}. Wrap any value that might contain quotes as
{{ json .nodes.fetch.body.name }} so the surrounding JSON stays valid.

Use only the node types above. If the request needs something else, get as close
as possible with http.request and say so in the description.`

// GenerateWorkflow turns a description into a proposed workflow.
//
// The graph is checked with Validate, not Runnable: this is a draft heading for
// a canvas, and the same half-finished states a human is allowed to save are
// allowed here. What it must not be is malformed — a duplicate id or an edge
// into nothing would break the editor rather than the run.
func (s *Service) GenerateWorkflow(ctx context.Context, workspaceID, prompt string) (Generated, error) {
	// The deadline belongs here rather than on the HTTP client, because this is
	// the caller that answers a request: it has to fail before the API server's
	// 30s write timeout, or it produces a response nobody is listening for. It
	// covers the retry too — two attempts spilling past that window would leave
	// the caller with a dead connection instead of an error.
	//
	// ponytail: generation is synchronous, so it lives inside one HTTP response
	// and the retry is squeezed into that. If it needs longer, the upgrade is
	// the 202-and-poll shape executions already use, not a bigger number here.
	ctx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()

	req := Request{
		System:    systemPrompt,
		Prompt:    prompt,
		MaxTokens: generateTokens,
		JSON:      true,
	}

	// One retry, with the reason. Models get JSON structurally right far more
	// often on a second pass when told what was wrong, and the alternative is
	// handing the user a failure they can do nothing about.
	for attempt := range 2 {
		response, err := s.Complete(ctx, workspaceID, req)
		if err != nil {
			return Generated{}, err
		}

		generated, err := parseGenerated(response.Text)
		if err == nil {
			return generated, nil
		}
		if attempt == 1 {
			return Generated{}, err
		}
		req.Prompt = fmt.Sprintf("%s\n\nYour previous answer was rejected: %v\nReturn a corrected JSON object.", prompt, err)
	}
	// Unreachable: the loop returns on both attempts.
	return Generated{}, ErrBadGraph
}

func parseGenerated(text string) (Generated, error) {
	var generated Generated
	if err := json.Unmarshal([]byte(stripFence(text)), &generated); err != nil {
		return Generated{}, fmt.Errorf("%w: %v", ErrBadGraph, err)
	}
	if err := generated.Graph.Validate(); err != nil {
		return Generated{}, fmt.Errorf("%w: %v", ErrBadGraph, err)
	}
	// Validate lets an empty graph through on purpose — an empty canvas is a
	// legitimate thing for a person to save. It is not a legitimate thing for a
	// model to answer with: {"name": "deploy on merge"} and nothing else would
	// otherwise be a 200 that opens a blank canvas, which reads as our bug.
	if len(generated.Graph.Nodes) == 0 {
		return Generated{}, fmt.Errorf("%w: the graph has no nodes", ErrBadGraph)
	}
	if strings.TrimSpace(generated.Name) == "" {
		generated.Name = "Untitled workflow"
	}
	layout(&generated.Graph)
	return generated, nil
}

// stripFence unwraps a ```json block. response_format asks for a bare object
// and most models comply, but one that wraps it has still answered correctly
// and rejecting that would be pedantry the user pays for.
func stripFence(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	if _, after, found := strings.Cut(trimmed, "\n"); found {
		trimmed = after
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(trimmed), "```"))
}

// layout spaces the nodes out when the model gave no positions. Without this
// they all land at 0,0 and the canvas opens with one node visible and the rest
// stacked underneath it.
func layout(graph *workflow.Graph) {
	for _, node := range graph.Nodes {
		if node.Position.X != 0 || node.Position.Y != 0 {
			return
		}
	}
	for i := range graph.Nodes {
		graph.Nodes[i].Position = workflow.Position{X: float64(i) * 280, Y: 0}
	}
}
