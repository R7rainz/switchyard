package ai

import (
	"encoding/json"
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
