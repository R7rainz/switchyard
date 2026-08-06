package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/execution"
)

// The engine hands a node its interpolated data and expects JSON back.
func TestPromptRunner(t *testing.T) {
	provider := &stubProvider{replies: []string{"a short answer"}}
	runners := Runners(NewService(provider, stubCreds{key: "k"}))

	runner, ok := runners["ai.prompt"]
	if !ok {
		t.Fatal("ai.prompt is not registered")
	}

	result, err := runner.Run(t.Context(), execution.Input{
		Data:        json.RawMessage(`{"prompt":"summarise this","system":"be brief"}`),
		WorkspaceID: "ws",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var output struct{ Text string }
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("output is not JSON: %s", result.Output)
	}
	if output.Text != "a short answer" {
		t.Fatalf("text = %q", output.Text)
	}
}

func TestPromptRunnerNeedsAPrompt(t *testing.T) {
	runners := Runners(NewService(&stubProvider{}, stubCreds{key: "k"}))
	_, err := runners["ai.prompt"].Run(t.Context(), execution.Input{Data: json.RawMessage(`{"label":"AI"}`)})
	if err == nil {
		t.Fatal("a node with no prompt must fail its run")
	}
}

func TestDedicatedAIRunners(t *testing.T) {
	provider := &stubProvider{replies: []string{
		"hello",
		"short summary",
		`{"label":"bug","reasoning":"it reports a failure"}`,
		`{"label":"bug","reasoning":"it reports a failure"}`,
		`{"decision":"true","reasoning":"the branch matches"}`,
	}}
	runners := Runners(NewService(provider, stubCreds{key: "k"}))

	for _, typeName := range []string{"ai.chat", "ai.summarize", "ai.classification", "ai.decision"} {
		if _, ok := runners[typeName]; !ok {
			t.Fatalf("%s is not registered", typeName)
		}
	}

	result, err := runners["ai.chat"].Run(t.Context(), execution.Input{Data: json.RawMessage(`{"prompt":"hello"}`), WorkspaceID: "ws"})
	if err != nil || !jsonContains(result.Output, `"text":"hello"`) {
		t.Fatalf("chat result = %s, err = %v", result.Output, err)
	}
	result, err = runners["ai.summarize"].Run(t.Context(), execution.Input{Data: json.RawMessage(`{"text":"a long document"}`), WorkspaceID: "ws"})
	if err != nil || !jsonContains(result.Output, `"text":"short summary"`) {
		t.Fatalf("summary result = %s, err = %v", result.Output, err)
	}
	result, err = runners["ai.classification"].Run(t.Context(), execution.Input{Data: json.RawMessage(`{"text":"the test failed","labels":["bug","feature"]}`), WorkspaceID: "ws"})
	if err != nil || !jsonContains(result.Output, `"label":"bug"`) {
		t.Fatalf("classification result = %s, err = %v", result.Output, err)
	}
	result, err = runners["ai.classification"].Run(t.Context(), execution.Input{Data: json.RawMessage(`{"text":"the test failed","labels":"[\"bug\",\"feature\"]"}`), WorkspaceID: "ws"})
	if err != nil || !jsonContains(result.Output, `"label":"bug"`) {
		t.Fatalf("classification editable JSON result = %s, err = %v", result.Output, err)
	}
	result, err = runners["ai.decision"].Run(t.Context(), execution.Input{Data: json.RawMessage(`{"question":"is this ready?"}`), WorkspaceID: "ws"})
	if err != nil || result.Branch != "true" {
		t.Fatalf("decision result = %s, branch = %q, err = %v", result.Output, result.Branch, err)
	}

	if provider.requests[2].JSONSchema == nil || provider.requests[4].JSONSchema == nil {
		t.Fatal("structured AI nodes did not request JSON schemas")
	}
}

func TestDedicatedAIRunnersRejectInvalidStructuredOutput(t *testing.T) {
	provider := &stubProvider{replies: []string{`{"label":"other","reasoning":"no"}`, `{"decision":"maybe","reasoning":"no"}`}}
	runners := Runners(NewService(provider, stubCreds{key: "k"}))

	if _, err := runners["ai.classification"].Run(t.Context(), execution.Input{Data: json.RawMessage(`{"text":"x","labels":["bug","feature"]}`)}); err == nil {
		t.Fatal("classification accepted an unknown label")
	}
	if _, err := runners["ai.decision"].Run(t.Context(), execution.Input{Data: json.RawMessage(`{"question":"x"}`)}); err == nil {
		t.Fatal("decision accepted an unknown branch")
	}
}

func jsonContains(raw json.RawMessage, fragment string) bool {
	return strings.Contains(string(raw), fragment)
}
