package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalid means the document itself is broken: duplicate ids, an edge to a
// node that does not exist, something oversize. A graph like this is refused on
// the way in, because storing it would mean storing corruption.
//
// Unlike an authorization failure, the detail is safe to return: the caller
// wrote the request, so being told what is wrong with it teaches them nothing
// about anyone else and saves them guessing.
var ErrInvalid = errors.New("workflow: invalid")

// ErrNotRunnable means the graph is intact but cannot execute — no trigger, a
// cycle, a node nothing reaches.
//
// This is deliberately not a save-time error. A builder canvas spends most of
// its life in exactly this state: a node dropped but not yet wired, a trigger
// about to be added. Rejecting the save would mean autosave failing while
// somebody is halfway through a thought. The check belongs where running does.
var ErrNotRunnable = errors.New("workflow: not runnable")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func notRunnable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrNotRunnable, fmt.Sprintf(format, args...))
}

// Graph is what the builder draws and the engine will walk: nodes and the
// edges between them, and nothing about how either behaves.
//
// The field names are React Flow's, on purpose. The frontend holds exactly this
// shape in useNodesState and useEdgesState, so a save is the array it already
// has rather than a translation of it — and a translation layer is somewhere
// for the two representations to drift apart.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Node is one step. Data is deliberately opaque here — it carries the label the
// canvas draws and whatever configuration the node type needs, and only the
// engine knows what a valid configuration looks like. Keeping it as raw JSON
// means this package can check the shape of a workflow without pretending to
// know what an HTTP node or an AI node requires, and an unrecognised field
// survives a save and load instead of being silently dropped.
type Node struct {
	ID       string          `json:"id"`
	Type     NodeType        `json:"type"`
	Position Position        `json:"position"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// Position is where the node sits on the canvas. It is presentation, carried
// so the builder can reopen a workflow the way it was left; nothing here reads
// it and the engine never will.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Edge connects one node's output to another's input. SourceHandle names which
// output it leaves from, so a condition node can have a "true" edge and a
// "false" edge; an empty handle is the default single output.
type Edge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"sourceHandle,omitempty"`
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
	"email":    true,
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

// Validate is the save-time gate, and it checks one thing: that the document is
// intact. Ids are unique, edges point at nodes that exist, nothing is absurdly
// large.
//
// It deliberately does not ask whether the graph could run. A canvas under
// construction is half-finished by definition, and a save is a draft rather
// than a promise to execute — Runnable is the check with an opinion about that,
// and the execution service calls it.
func (g Graph) Validate() error {
	if len(g.Nodes) > maxNodes {
		return invalid("a workflow is limited to %d nodes", maxNodes)
	}
	if len(g.Edges) > maxEdges {
		return invalid("a workflow is limited to %d edges", maxEdges)
	}

	nodes := make(map[string]struct{}, len(g.Nodes))
	for _, node := range g.Nodes {
		if err := checkID("node", node.ID); err != nil {
			return err
		}
		if _, clash := nodes[node.ID]; clash {
			// Two nodes with one id makes every edge touching it ambiguous,
			// and which one wins would come down to slice order. React Flow
			// misbehaves the same way, so this is broken for the canvas too.
			return invalid("duplicate node id %q", node.ID)
		}
		if node.Data != nil && !json.Valid(node.Data) {
			return invalid("node %q has malformed data", node.ID)
		}
		nodes[node.ID] = struct{}{}
	}

	seen := make(map[string]struct{}, len(g.Edges))
	for _, edge := range g.Edges {
		if err := checkID("edge", edge.ID); err != nil {
			return err
		}
		if _, clash := seen[edge.ID]; clash {
			return invalid("duplicate edge id %q", edge.ID)
		}
		seen[edge.ID] = struct{}{}

		// A dangling endpoint is what a deleted node leaves behind, and it is
		// corruption rather than an unfinished thought: React Flow will not
		// draw the edge and the engine would trip over it.
		if _, ok := nodes[edge.Source]; !ok {
			return invalid("edge %q starts at unknown node %q", edge.ID, edge.Source)
		}
		if _, ok := nodes[edge.Target]; !ok {
			return invalid("edge %q ends at unknown node %q", edge.ID, edge.Target)
		}
	}
	return nil
}

// Runnable reports whether this graph can actually execute. It is the set of
// guarantees the engine is allowed to assume, checked once before a run rather
// than rediscovered at every node.
//
// The execution service calls this; saving does not. A graph that fails here is
// a perfectly good draft.
func (g Graph) Runnable() error {
	// A stored graph has already passed Validate, but Runnable is cheap and an
	// assumption that only holds by convention is one that eventually does not.
	if err := g.Validate(); err != nil {
		return err
	}
	if len(g.Nodes) == 0 {
		return notRunnable("a workflow needs at least one node")
	}

	nodes := make(map[string]struct{}, len(g.Nodes))
	var trigger string

	for _, node := range g.Nodes {
		// Unknown types are a run-time question, not a save-time one: the
		// frontend may ship a node type before this list learns about it, and
		// that should cost a failed run rather than a failed save.
		if !categories[node.Type.Category()] {
			return notRunnable("node %q has unknown type %q", node.ID, node.Type)
		}
		if node.Type.IsTrigger() {
			if trigger != "" {
				// Two triggers means two possible starting points, so "what
				// ran" would depend on which one the engine picked.
				return notRunnable("a workflow runs from exactly one trigger, found %q and %q", trigger, node.ID)
			}
			trigger = node.ID
		}
		nodes[node.ID] = struct{}{}
	}

	if trigger == "" {
		return notRunnable("a workflow needs a trigger node")
	}

	outgoing := make(map[string][]string, len(g.Nodes))
	for _, edge := range g.Edges {
		if edge.Target == trigger {
			// The trigger is where a run begins. An edge into it says
			// something happens before the beginning.
			return notRunnable("edge %q points back into the trigger", edge.ID)
		}
		outgoing[edge.Source] = append(outgoing[edge.Source], edge.Target)
	}

	if err := checkAcyclic(nodes, g.Edges); err != nil {
		return err
	}
	return checkReachable(g.Nodes, outgoing, trigger)
}

// checkAcyclic refuses back-edges, by Kahn's algorithm.
//
// Rejecting cycles is what lets the engine be a topological walk rather than a
// loop detector carrying a step budget. If looping is ever wanted it should be
// an explicit node type with a visible bound, not an accidental edge that runs
// forever because someone dragged one connector too far.
func checkAcyclic(nodes map[string]struct{}, edges []Edge) error {
	indegree := make(map[string]int, len(nodes))
	successors := make(map[string][]string, len(nodes))
	for _, edge := range edges {
		indegree[edge.Target]++
		successors[edge.Source] = append(successors[edge.Source], edge.Target)
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
		return notRunnable("the graph has a cycle")
	}
	return nil
}

// checkReachable refuses orphans. A node the trigger cannot reach can never
// run, so it would sit in the execution view as a step that is permanently
// pending — fine on a canvas someone is still building, not fine in a run.
func checkReachable(nodes []Node, outgoing map[string][]string, trigger string) error {
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
			return notRunnable("node %q cannot be reached from the trigger", node.ID)
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
