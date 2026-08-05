package ai

import (
	_ "embed"
	"encoding/json"
	"testing"
)

//go:embed testdata/workflow_evaluations.json
var workflowEvaluationData []byte

type workflowEvaluationFixture struct {
	Name         string `json:"name"`
	Prompt       string `json:"prompt"`
	Response     string `json:"response"`
	WantValid    bool   `json:"valid"`
	WantRunnable bool   `json:"runnable"`
}

// These fixtures are the first offline baseline for model changes. They keep
// graph validity separate from runnability: a draft may be valid to edit, but
// an AI proposal should normally be runnable before it reaches the canvas.
func TestWorkflowEvaluationFixtures(t *testing.T) {
	var fixtures []workflowEvaluationFixture
	if err := json.Unmarshal(workflowEvaluationData, &fixtures); err != nil {
		t.Fatalf("decode evaluation fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("evaluation fixtures are empty")
	}

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			generated, err := parseGenerated(fixture.Response)
			valid := err == nil
			if valid != fixture.WantValid {
				t.Fatalf("valid = %t, want %t (prompt %q): %v", valid, fixture.WantValid, fixture.Prompt, err)
			}
			if !valid {
				return
			}

			runnable := generated.Graph.Runnable() == nil
			if runnable != fixture.WantRunnable {
				t.Fatalf("runnable = %t, want %t (prompt %q)", runnable, fixture.WantRunnable, fixture.Prompt)
			}
		})
	}
}
