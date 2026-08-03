package workflow

import (
	"errors"
	"strings"
	"testing"
)

// node and edge keep the tables below readable; positions are irrelevant to
// every rule here.
func node(id string, nodeType NodeType) Node { return Node{ID: id, Type: nodeType} }
func edge(id, from, to string) Edge          { return Edge{ID: id, From: from, To: to} }

// validGraph is the smallest thing that passes: trigger -> one step.
func validGraph() Graph {
	return Graph{
		Nodes: []Node{node("t", "trigger.manual"), node("a", "http.request")},
		Edges: []Edge{edge("e1", "t", "a")},
	}
}

func TestValidateAccepts(t *testing.T) {
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
				{ID: "e2", From: "cond", To: "yes", Branch: "true"},
				{ID: "e3", From: "cond", To: "no", Branch: "false"},
				edge("e4", "yes", "end"),
				edge("e5", "no", "end"),
			},
		},
		"config is opaque, not inspected": {
			Nodes: []Node{
				node("t", "trigger.manual"),
				{ID: "a", Type: "ai.prompt", Config: []byte(`{"whatever":[1,2,3]}`)},
			},
			Edges: []Edge{edge("e1", "t", "a")},
		},
	}

	for name, graph := range cases {
		t.Run(name, func(t *testing.T) {
			if err := graph.Validate(); err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
		})
	}
}

func TestValidateRejects(t *testing.T) {
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
		"malformed config": {
			graph: Graph{Nodes: []Node{
				{ID: "t", Type: "trigger.manual", Config: []byte(`{not json`)},
			}},
			want: "malformed config",
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
