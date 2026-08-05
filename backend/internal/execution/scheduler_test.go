package execution

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

func TestCronMatchesExactAndWildcardMinutes(t *testing.T) {
	now := time.Date(2026, time.August, 5, 9, 30, 0, 0, time.UTC)
	if !cronMatches("30 9 * * *", now) {
		t.Fatal("exact schedule did not match")
	}
	if cronMatches("31 9 * * *", now) {
		t.Fatal("wrong minute matched")
	}
	if !cronMatches("* * * * *", now) {
		t.Fatal("wildcard schedule did not match")
	}
	if !cronMatches("*/15 9 * * 1-5", now) {
		t.Fatal("weekday range schedule did not match")
	}
	if cronMatches("30 9 * * 0", now) {
		t.Fatal("Sunday schedule matched a Wednesday")
	}
	if !cronMatches("30 9 5,6,7 * *", now) {
		t.Fatal("day list schedule did not match")
	}
}

func TestSchedulerStartsMatchingWorkflow(t *testing.T) {
	workflows := workflow.NewService(workflow.NewMemoryStore())
	created, err := workflows.Create(context.Background(), "ws", "user", "scheduled", "", workflow.Graph{
		Nodes: []workflow.Node{{
			ID:   "trigger",
			Type: "trigger.schedule",
			Data: json.RawMessage(`{"cron":"* * * * *"}`),
		}},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	runs := NewMemoryStore()
	svc := NewService(runs, workflows, Builtin(nil), Options{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.StartScheduler(ctx, time.Millisecond)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		found, err := runs.List(ctx, "ws", created.ID, 1)
		if err != nil {
			t.Fatalf("list executions: %v", err)
		}
		if len(found) == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("scheduler did not start a matching workflow")
}
