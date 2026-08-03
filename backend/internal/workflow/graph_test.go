package workflow

import (
	"errors"
	"strings"
	"testing"
)

// node and edge keep the tables below readable; positions are irrelevant to
// every rule here.
func node(id string, nodeType NodeType) Node { return Node{ID: id, Type: nodeType} }
func edge(id, source, target string) Edge    { return Edge{ID: id, Source: source, Target: target} }

// validGraph is the smallest thing that both saves and runs: trigger -> step.
func validGraph() Graph {
	return Graph{
		Nodes: []Node{node("t", "trigger.manual"), node("a", "http.request")},
		Edges: []Edge{edge("e1", "t", "a")},
	}
}

// Everything a canvas can be mid-edit has to save. These are the states a
// builder passes through on the way to something runnable, and rejecting them
// would mean autosave failing while somebody is halfway through a thought.
func TestValidateAcceptsDrafts(t *testing.T) {
	cases := map[string]Graph{
		"empty canvas":     {},
		"one loose node":   {Nodes: []Node{node("a", "http.request")}},
		"no trigger yet":   {Nodes: []Node{node("a", "http.request"), node("b", "slack.message")}},
		"two triggers":     {Nodes: []Node{node("t1", "trigger.manual"), node("t2", "trigger.webhook")}},
		"unknown type":     {Nodes: []Node{node("a", "kubernetes.apply")}},
		"unreachable node": {Nodes: []Node{node("t", "trigger.manual"), node("lonely", "slack.message")}},
		"a cycle": {
			Nodes: []Node{node("t", "trigger.manual"), node("a", "http.request")},
			Edges: []Edge{edge("e1", "t", "a"), edge("e2", "a", "t")},
		},
		"finished": validGraph(),
	}

	for name, graph := range cases {
		t.Run(name, func(t *testing.T) {
			if err := graph.Validate(); err != nil {
				t.Fatalf("a draft must still save, got %v", err)
			}
		})
	}
}

// Corruption is a different thing from an unfinished draft, and it is refused
// on the way in: storing it would mean storing something React Flow cannot
// draw and the engine cannot read.
func TestValidateRejectsCorruption(t *testing.T) {
	cases := map[string]struct {
		graph Graph
		want  string
	}{
		"empty node id": {
			graph: Graph{Nodes: []Node{node("  ", "trigger.manual")}},
			want:  "every node needs an id",
		},
		"duplicate node id": {
			graph: Graph{Nodes: []Node{
				node("t", "trigger.manual"),
				node("t", "http.request"),
			}},
			want: "duplicate node id",
		},
		"malformed data": {
			graph: Graph{Nodes: []Node{
				{ID: "t", Type: "trigger.manual", Data: []byte(`{not json`)},
			}},
			want: "malformed data",
		},
		"edge from nowhere": {
			graph: Graph{
				Nodes: []Node{node("t", "trigger.manual")},
				Edges: []Edge{edge("e1", "ghost", "t")},
			},
			want: "starts at unknown node",
		},
		"edge to nowhere": {
			graph: Graph{
				Nodes: []Node{node("t", "trigger.manual")},
				Edges: []Edge{edge("e1", "t", "ghost")},
			},
			want: "ends at unknown node",
		},
		"duplicate edge id": {
			graph: Graph{
				Nodes: []Node{node("t", "trigger.manual"), node("a", "http.request")},
				Edges: []Edge{edge("e1", "t", "a"), edge("e1", "t", "a")},
			},
			want: "duplicate edge id",
		},
		"node id too long": {
			graph: Graph{Nodes: []Node{node(strings.Repeat("x", maxIDLen+1), "trigger.manual")}},
			want:  "longer than",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.graph.Validate()
			if err == nil {
				t.Fatal("expected rejection, got nil")
			}
			// Every rejection has to be recognisable to the API layer, or it
			// answers 500 for a graph the caller could have fixed.
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error does not wrap ErrInvalid: %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestRunnableAccepts(t *testing.T) {
	cases := map[string]Graph{
		"trigger and one step": validGraph(),
		"trigger alone": {
			Nodes: []Node{node("t", "trigger.webhook")},
		},
		"branching then merging": {
			Nodes: []Node{
				node("t", "trigger.manual"),
				node("cond", "logic.condition"),
				node("yes", "slack.message"),
				node("no", "discord.message"),
				node("end", "variable.set"),
			},
			Edges: []Edge{
				edge("e1", "t", "cond"),
				{ID: "e2", Source: "cond", Target: "yes", SourceHandle: "true"},
				{ID: "e3", Source: "cond", Target: "no", SourceHandle: "false"},
				edge("e4", "yes", "end"),
				edge("e5", "no", "end"),
			},
		},
		"data is opaque, not inspected": {
			Nodes: []Node{
				node("t", "trigger.manual"),
				{ID: "a", Type: "ai.prompt", Data: []byte(`{"label":"summarise","whatever":[1,2,3]}`)},
			},
			Edges: []Edge{edge("e1", "t", "a")},
		},
	}

	for name, graph := range cases {
		t.Run(name, func(t *testing.T) {
			if err := graph.Runnable(); err != nil {
				t.Fatalf("expected runnable, got %v", err)
			}
		})
	}
}

// The guarantees the engine gets to assume. Each of these saves fine and fails
// here instead.
func TestRunnableRejects(t *testing.T) {
	cases := map[string]struct {
		graph Graph
		want  string
	}{
		"no nodes": {
			graph: Graph{},
			want:  "at least one node",
		},
		"no trigger": {
			graph: Graph{Nodes: []Node{node("a", "http.request")}},
			want:  "needs a trigger",
		},
		"two triggers": {
			graph: Graph{Nodes: []Node{
				node("t1", "trigger.manual"),
				node("t2", "trigger.webhook"),
			}},
			want: "exactly one trigger",
		},
		"unknown category": {
			graph: Graph{Nodes: []Node{
				node("t", "trigger.manual"),
				node("a", "kubernetes.apply"),
			}},
			want: "unknown type",
		},
		"type without an action": {
			graph: Graph{Nodes: []Node{node("t", "trigger")}},
			want:  "unknown type",
		},
		"edge into the trigger": {
			graph: Graph{
				Nodes: []Node{node("t", "trigger.manual"), node("a", "http.request")},
				Edges: []Edge{edge("e1", "t", "a"), edge("e2", "a", "t")},
			},
			want: "points back into the trigger",
		},
		"self loop": {
			graph: Graph{
				Nodes: []Node{node("t", "trigger.manual"), node("a", "http.request")},
				Edges: []Edge{edge("e1", "t", "a"), edge("e2", "a", "a")},
			},
			want: "cycle",
		},
		"cycle away from the trigger": {
			graph: Graph{
				Nodes: []Node{
					node("t", "trigger.manual"),
					node("a", "http.request"),
					node("b", "http.request"),
					node("c", "http.request"),
				},
				Edges: []Edge{
					edge("e1", "t", "a"),
					edge("e2", "a", "b"),
					edge("e3", "b", "c"),
					edge("e4", "c", "a"),
				},
			},
			want: "cycle",
		},
		"orphan node": {
			graph: Graph{
				Nodes: []Node{
					node("t", "trigger.manual"),
					node("a", "http.request"),
					node("lonely", "slack.message"),
				},
				Edges: []Edge{edge("e1", "t", "a")},
			},
			want: "cannot be reached",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// It saves. That is the point of the split.
			if err := tc.graph.Validate(); err != nil {
				t.Fatalf("this should still be a savable draft, got %v", err)
			}

			err := tc.graph.Runnable()
			if err == nil {
				t.Fatal("expected rejection, got nil")
			}
			if !errors.Is(err, ErrNotRunnable) {
				t.Fatalf("error does not wrap ErrNotRunnable: %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// Corruption stays corruption even when it is Runnable being asked, so the
// engine never has to re-check what the store already promised.
func TestRunnableStillCatchesCorruption(t *testing.T) {
	graph := Graph{
		Nodes: []Node{node("t", "trigger.manual")},
		Edges: []Edge{edge("e1", "t", "ghost")},
	}
	if err := graph.Runnable(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

// The limits exist so one save cannot hand the engine an unbounded walk.
func TestValidateRejectsOversizeGraphs(t *testing.T) {
	nodes := make([]Node, 0, maxNodes+1)
	nodes = append(nodes, node("t", "trigger.manual"))
	for i := range maxNodes {
		nodes = append(nodes, node(string(rune('a'+i%26))+string(rune(i)), "http.request"))
	}

	err := Graph{Nodes: nodes}.Validate()
	if err == nil || !strings.Contains(err.Error(), "limited to") {
		t.Fatalf("got %v, want a size limit rejection", err)
	}
}

func TestCategory(t *testing.T) {
	cases := map[NodeType]struct {
		category  string
		isTrigger bool
	}{
		"trigger.manual": {"trigger", true},
		"http.request":   {"http", false},
		"a.b.c":          {"a", false}, // only the first dot separates
		"trigger":        {"", false},
		"trigger.":       {"", false},
		".manual":        {"", false},
		"":               {"", false},
	}

	for nodeType, want := range cases {
		if got := nodeType.Category(); got != want.category {
			t.Errorf("%q: category = %q, want %q", nodeType, got, want.category)
		}
		if got := nodeType.IsTrigger(); got != want.isTrigger {
			t.Errorf("%q: IsTrigger = %v, want %v", nodeType, got, want.isTrigger)
		}
	}
}
