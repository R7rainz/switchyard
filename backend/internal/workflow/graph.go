package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalid is every way a workflow can be rejected for something the caller
// wrote — a bad graph, a missing name, an oversize description. The wrapped
// message says which one.
//
// Unlike an authorization failure, the detail is safe to return: the caller
// wrote the request, so being told what is wrong with it teaches them nothing
// about anyone else and saves them guessing.
var ErrInvalid = errors.New("workflow: invalid")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Graph is what the builder draws and the engine will walk: nodes and the
// edges between them, and nothing about how either behaves.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Node is one step. Config is deliberately opaque here — what a valid
// configuration looks like depends on the node type, and only the engine knows
// that. Keeping it as raw JSON means this package can validate the shape of a
// workflow without pretending to know what an HTTP node or an AI node needs,
// and an unrecognised field survives a save/load round trip instead of being
// silently dropped.
type Node struct {
	ID       string          `json:"id"`
	Type     NodeType        `json:"type"`
	Name     string          `json:"name,omitempty"`
	Position Position        `json:"position"`
	Config   json.RawMessage `json:"config,omitempty"`
}

// Position is where the node sits on the canvas. It is presentation, carried
// so the builder can reopen a workflow the way it was left; nothing here reads
// it and the engine never will.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Edge connects one node's output to another's input. Branch names which
// output it leaves from, so a condition node can have a "true" edge and a
// "false" edge; an empty branch is the default single output.
type Edge struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Branch string `json:"branch,omitempty"`
}

// NodeType is "category.action" — "http.request", "logic.condition".
type NodeType string

// categories are the node families the MVP covers.
//
// Only the category is checked, not the action. This package needs to know
// which nodes are triggers, because the one-trigger rule depends on it, and
// that is genuinely all it can know: whether "http.request" is a real action
// is the node registry's question, and the registry ships with the engine.
// Guessing the action list here would mean stored graphs disagreeing with the
// engine's list later, which is a migration rather than a validation.
var categories = map[string]bool{
	"trigger":  true,
	"logic":    true,
	"ai":       true,
	"http":     true,
	"github":   true,
	"slack":    true,
	"discord":  true,
	"variable": true,
}

// Category is the part before the dot, or "" if the type is not in that shape.
func (t NodeType) Category() string {
	category, action, ok := strings.Cut(string(t), ".")
	if !ok || category == "" || action == "" {
		return ""
	}
	return category
}

// IsTrigger reports whether this node is where an execution starts.
func (t NodeType) IsTrigger() bool { return t.Category() == "trigger" }

// A graph is bounded so a single save cannot cost the engine an unbounded
// walk. These are far above any workflow a person draws by hand.
const (
	maxNodes = 500
	maxEdges = 2000
	maxIDLen = 128
)

// Validate is the gate every graph passes before it is stored, and the reason
// this package exists. Everything downstream — the engine especially — is
// allowed to assume these hold, so each rule here is a check the engine does
// not have to repeat at every node.
func (g Graph) Validate() error {
	if len(g.Nodes) == 0 {
		return invalid("a workflow needs at least one node")
	}
	if len(g.Nodes) > maxNodes {
		return invalid("a workflow is limited to %d nodes", maxNodes)
	}
	if len(g.Edges) > maxEdges {
		return invalid("a workflow is limited to %d edges", maxEdges)
	}

	nodes := make(map[string]Node, len(g.Nodes))
	var trigger string

	for _, node := range g.Nodes {
		if err := checkID("node", node.ID); err != nil {
			return err
		}
		if _, clash := nodes[node.ID]; clash {
			// Two nodes with one id makes every edge touching it ambiguous,
			// and which one wins would come down to slice order.
			return invalid("duplicate node id %q", node.ID)
		}
		if !categories[node.Type.Category()] {
			return invalid("node %q has unknown type %q", node.ID, node.Type)
		}
		if node.Config != nil && !json.Valid(node.Config) {
			return invalid("node %q has malformed config", node.ID)
		}

		if node.Type.IsTrigger() {
			if trigger != "" {
				// Two triggers means two possible starting points, so "what
				// ran" would depend on which one the engine picked.
				return invalid("a workflow has exactly one trigger, found %q and %q", trigger, node.ID)
			}
			trigger = node.ID
		}
		nodes[node.ID] = node
	}

	if trigger == "" {
		return invalid("a workflow needs a trigger node")
	}

	outgoing, err := g.checkEdges(nodes, trigger)
	if err != nil {
		return err
	}
	if err := checkAcyclic(nodes, g.Edges); err != nil {
		return err
	}
	return checkReachable(nodes, outgoing, trigger)
}

// checkEdges validates every edge and returns the adjacency list the later
// passes walk, so the graph is only turned into a map once.
func (g Graph) checkEdges(nodes map[string]Node, trigger string) (map[string][]string, error) {
	seen := make(map[string]bool, len(g.Edges))
	outgoing := make(map[string][]string, len(nodes))

	for _, edge := range g.Edges {
		if err := checkID("edge", edge.ID); err != nil {
			return nil, err
		}
		if seen[edge.ID] {
			return nil, invalid("duplicate edge id %q", edge.ID)
		}
		seen[edge.ID] = true

		// A dangling endpoint is the common result of deleting a node in the
		// builder and shipping the edges that pointed at it. The engine would
		// hit it mid-run, so it is refused at save time instead.
		if _, ok := nodes[edge.From]; !ok {
			return nil, invalid("edge %q starts at unknown node %q", edge.ID, edge.From)
		}
		if _, ok := nodes[edge.To]; !ok {
			return nil, invalid("edge %q ends at unknown node %q", edge.ID, edge.To)
		}
		if edge.To == trigger {
			// The trigger is where a run begins. An edge into it says
			// something happens before the beginning.
			return nil, invalid("edge %q points back into the trigger", edge.ID)
		}

		outgoing[edge.From] = append(outgoing[edge.From], edge.To)
	}
	return outgoing, nil
}

// checkAcyclic refuses back-edges, by Kahn's algorithm.
//
// Rejecting cycles is what lets the engine be a topological walk rather than a
// loop detector carrying a step budget. If looping is ever wanted it should be
// an explicit node type with a visible bound, not an accidental edge that runs
// forever because someone dragged one connector too far.
func checkAcyclic(nodes map[string]Node, edges []Edge) error {
	indegree := make(map[string]int, len(nodes))
	successors := make(map[string][]string, len(nodes))
	for _, edge := range edges {
		indegree[edge.To]++
		successors[edge.From] = append(successors[edge.From], edge.To)
	}

	queue := make([]string, 0, len(nodes))
	for id := range nodes {
		if indegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	settled := 0
	for len(queue) > 0 {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		settled++

		for _, next := range successors[id] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	// Anything left is in a cycle or downstream of one: a node in a cycle
	// never reaches indegree zero, so it is never settled.
	if settled != len(nodes) {
		return invalid("the graph has a cycle")
	}
	return nil
}

// checkReachable refuses orphans. A node the trigger cannot reach can never
// run, so it is either a mistake or a leftover, and either way it would sit in
// the execution view as a step that is permanently pending.
func checkReachable(nodes map[string]Node, outgoing map[string][]string, trigger string) error {
	seen := map[string]bool{trigger: true}
	queue := []string{trigger}

	for len(queue) > 0 {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, next := range outgoing[id] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}

	for _, node := range nodes {
		if !seen[node.ID] {
			return invalid("node %q cannot be reached from the trigger", node.ID)
		}
	}
	return nil
}

func checkID(kind, id string) error {
	if strings.TrimSpace(id) == "" {
		return invalid("every %s needs an id", kind)
	}
	if len(id) > maxIDLen {
		return invalid("%s id %q is longer than %d characters", kind, id, maxIDLen)
	}
	return nil
}
