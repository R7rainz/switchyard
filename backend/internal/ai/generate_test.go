package ai

import (
	"errors"
	"strings"
	"testing"
)

const goodGraph = `{"name":"ship it","description":"posts on merge","graph":{
	"nodes":[
		{"id":"t","type":"trigger.manual","position":{"x":0,"y":0},"data":{"label":"Start"}},
		{"id":"a","type":"http.request","position":{"x":280,"y":0},"data":{"label":"Call","url":"https://example.com"}}
	],
	"edges":[{"id":"e1","source":"t","target":"a"}]}}`

func TestGenerateWorkflow(t *testing.T) {
	provider := &stubProvider{replies: []string{goodGraph}}
	service := NewService(provider, stubCreds{key: "sk-test"})

	generated, err := service.GenerateWorkflow(t.Context(), "ws", "post to slack when main merges")
	if err != nil {
		t.Fatalf("GenerateWorkflow: %v", err)
	}
	if generated.Name != "ship it" {
		t.Fatalf("name = %q", generated.Name)
	}
	if len(generated.Graph.Nodes) != 2 || len(generated.Graph.Edges) != 1 {
		t.Fatalf("graph did not survive: %+v", generated.Graph)
	}
	if provider.keys[0] != "sk-test" {
		t.Fatalf("the workspace's key did not reach the provider: %q", provider.keys[0])
	}
	if provider.requests[0].JSONSchema == nil || provider.requests[0].JSONSchema.Name != "switchyard_workflow" {
		t.Fatalf("generation did not request the workflow schema: %+v", provider.requests[0].JSONSchema)
	}
	if !provider.requests[0].JSONSchema.Strict {
		t.Fatal("generation schema should be strict")
	}
}

// A model that answers with something unusable gets one more go, told what was
// wrong. Without this the user sees a failure they cannot act on.
func TestGenerateRetriesOnceWithTheReason(t *testing.T) {
	broken := `{"name":"x","graph":{"nodes":[{"id":"t","type":"trigger.manual"}],"edges":[{"id":"e1","source":"t","target":"ghost"}]}}`
	provider := &stubProvider{replies: []string{broken, goodGraph}}
	service := NewService(provider, stubCreds{key: "sk-test"})

	if _, err := service.GenerateWorkflow(t.Context(), "ws", "do a thing"); err != nil {
		t.Fatalf("the retry should have succeeded: %v", err)
	}
	if len(provider.prompts) != 2 {
		t.Fatalf("called the model %d times, want 2", len(provider.prompts))
	}
	if !strings.Contains(provider.prompts[1], "ghost") {
		t.Fatalf("the retry did not carry the reason: %q", provider.prompts[1])
	}
}

// Two bad answers is a failure, not a third call.
func TestGenerateGivesUpAfterTwo(t *testing.T) {
	provider := &stubProvider{replies: []string{"not json", "still not json"}}
	service := NewService(provider, stubCreds{key: "sk-test"})

	_, err := service.GenerateWorkflow(t.Context(), "ws", "do a thing")
	if !errors.Is(err, ErrBadGraph) {
		t.Fatalf("err = %v, want ErrBadGraph", err)
	}
	if len(provider.prompts) != 2 {
		t.Fatalf("called the model %d times, want 2", len(provider.prompts))
	}
}

// A workspace with no key gets its own error, so the API can name the fix
// instead of returning a generic upstream failure.
func TestGenerateWithoutAKey(t *testing.T) {
	service := NewService(&stubProvider{}, stubCreds{})

	if _, err := service.GenerateWorkflow(t.Context(), "ws", "anything"); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("err = %v, want ErrNoCredential", err)
	}
}

// A model that returns every node at the origin would open a canvas with one
// node visible and the rest stacked under it.
func TestUnpositionedNodesAreLaidOut(t *testing.T) {
	flat := `{"name":"x","graph":{"nodes":[
		{"id":"t","type":"trigger.manual"},{"id":"a","type":"http.request"},{"id":"b","type":"http.request"}],
		"edges":[]}}`
	provider := &stubProvider{replies: []string{flat}}
	service := NewService(provider, stubCreds{key: "k"})

	generated, err := service.GenerateWorkflow(t.Context(), "ws", "x")
	if err != nil {
		t.Fatalf("GenerateWorkflow: %v", err)
	}
	seen := map[float64]bool{}
	for _, node := range generated.Graph.Nodes {
		if seen[node.Position.X] {
			t.Fatalf("two nodes share x = %v", node.Position.X)
		}
		seen[node.Position.X] = true
	}
}

func TestGeneratedGraphNormalizesMissingEdges(t *testing.T) {
	missing := `{"name":"manual","graph":{"nodes":[{"id":"t","type":"trigger.manual"}]}}`
	provider := &stubProvider{replies: []string{missing}}

	generated, err := NewService(provider, stubCreds{key: "k"}).GenerateWorkflow(t.Context(), "ws", "start manually")
	if err != nil {
		t.Fatalf("GenerateWorkflow: %v", err)
	}
	if generated.Graph.Edges == nil {
		t.Fatal("generated edges are nil, want an empty array")
	}
}

// Positions the model did give are left alone.
func TestGivenPositionsAreKept(t *testing.T) {
	provider := &stubProvider{replies: []string{goodGraph}}
	service := NewService(provider, stubCreds{key: "k"})

	generated, _ := service.GenerateWorkflow(t.Context(), "ws", "x")
	if generated.Graph.Nodes[1].Position.X != 280 {
		t.Fatalf("position was overwritten: %+v", generated.Graph.Nodes[1].Position)
	}
}

func TestStripFence(t *testing.T) {
	cases := map[string]string{
		"{\"a\":1}":                   `{"a":1}`,
		"```json\n{\"a\":1}\n```":     `{"a":1}`,
		"```\n{\"a\":1}\n```":         `{"a":1}`,
		"  \n```json\n{\"a\":1}```\n": `{"a":1}`,
	}
	for input, want := range cases {
		if got := stripFence(input); got != want {
			t.Errorf("stripFence(%q) = %q, want %q", input, got, want)
		}
	}
}

// A model that answers with a name and no graph must not be a success. Validate
// lets an empty graph through — an empty canvas is a legitimate thing for a
// person to save — so this is the one place that has to say otherwise, or
// generating opens a blank canvas and reads as our bug.
func TestGenerateRejectsAGraphWithNoNodes(t *testing.T) {
	cases := map[string]string{
		"no graph key":       `{"name":"deploy on merge","description":"ships main"}`,
		"empty graph":        `{"name":"deploy","graph":{"nodes":[],"edges":[]}}`,
		"prose in the graph": `{"name":"deploy","graph":{"nodes":null,"edges":null}}`,
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			provider := &stubProvider{replies: []string{reply, reply}}
			_, err := NewService(provider, stubCreds{key: "k"}).GenerateWorkflow(t.Context(), "ws", "x")
			if !errors.Is(err, ErrBadGraph) {
				t.Fatalf("err = %v, want ErrBadGraph", err)
			}
			if !strings.Contains(provider.prompts[1], "no nodes") {
				t.Fatalf("the retry did not say what was wrong: %q", provider.prompts[1])
			}
		})
	}
}
