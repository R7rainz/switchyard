package aifeedback

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

func testGraph(data string) workflow.Graph {
	return workflow.Graph{
		Nodes: []workflow.Node{
			{ID: "trigger", Type: "trigger.manual", Data: json.RawMessage(`{"label":"Start"}`)},
			{ID: "request", Type: "http.request", Data: json.RawMessage(data)},
		},
		Edges: []workflow.Edge{{ID: "edge", Source: "trigger", Target: "request"}},
	}
}

func TestSubmitRequiresValidOptInData(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(store)

	err := service.Submit(context.Background(), Submission{
		WorkspaceID:    "workspace",
		UserID:         "user",
		Prompt:         "call it with Bearer sk-test-secret",
		Outcome:        OutcomeAccepted,
		GeneratedGraph: testGraph(`{"label":"Call","authorization":"secret"}`),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	records := store.Records()
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	var graph map[string]any
	if err := json.Unmarshal(records[0].GeneratedGraph, &graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	nodes := graph["nodes"].([]any)
	data := nodes[1].(map[string]any)["data"].(map[string]any)
	if data["authorization"] != "[REDACTED]" {
		t.Fatalf("authorization = %v", data["authorization"])
	}
	if strings.Contains(records[0].Prompt, "Bearer sk-") || strings.Contains(records[0].Prompt, "sk-test-secret") {
		t.Fatal("prompt still contains the bearer credential")
	}
}

func TestSubmitRejectsInvalidOutcomeButAllowsDrafts(t *testing.T) {
	service := NewService(NewMemoryStore())

	if err := service.Submit(context.Background(), Submission{
		WorkspaceID:    "workspace",
		UserID:         "user",
		Prompt:         "build it",
		Outcome:        "later",
		GeneratedGraph: testGraph(`{"label":"Call"}`),
	}); err == nil {
		t.Fatal("invalid outcome was accepted")
	}

	if err := service.Submit(context.Background(), Submission{
		WorkspaceID:    "workspace",
		UserID:         "user",
		Prompt:         "build it",
		Outcome:        OutcomeRejected,
		GeneratedGraph: workflow.Graph{Nodes: []workflow.Node{{ID: "orphan", Type: "http.request"}}},
	}); err != nil {
		// A structurally valid draft does not need to be runnable, so this is
		// deliberately accepted. It is still useful negative feedback.
		t.Fatalf("valid draft rejected: %v", err)
	}

	if err := service.Submit(context.Background(), Submission{
		WorkspaceID:    "workspace",
		UserID:         "user",
		Prompt:         "build it",
		Outcome:        OutcomeRejected,
		GeneratedGraph: testGraph(`{"label":"Call"`),
	}); err == nil {
		t.Fatal("malformed graph was accepted")
	}
}
