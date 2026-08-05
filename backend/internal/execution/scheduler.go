package execution

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

// StartScheduler polls persisted workflow graphs. The process-local slot map
// prevents duplicate runs when the ticker fires more than once in a minute.
// ponytail: five-field numeric cron only; add named values/timezones when scheduling needs them.
func (s *Service) StartScheduler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		all, ok := s.workflows.(interface {
			ListAll(context.Context) ([]workflow.Workflow, error)
		})
		if !ok {
			return
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		seen := map[string]time.Time{}
		var mu sync.Mutex
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				workflows, err := all.ListAll(ctx)
				if err != nil {
					continue
				}
				slot := now.UTC().Truncate(time.Minute)
				for _, flow := range workflows {
					cron, ok := scheduleOf(flow.Graph)
					if !ok || !cronMatches(cron, slot) {
						continue
					}
					mu.Lock()
					duplicate := seen[flow.ID].Equal(slot)
					seen[flow.ID] = slot
					mu.Unlock()
					if !duplicate {
						_, _ = s.Start(ctx, flow.WorkspaceID, flow.ID, "", TriggerSchedule, nil)
					}
				}
			}
		}
	}()
}

func scheduleOf(graph workflow.Graph) (string, bool) {
	for _, node := range graph.Nodes {
		if node.Type != "trigger.schedule" {
			continue
		}
		var data struct {
			Cron string `json:"cron"`
		}
		if json.Unmarshal(node.Data, &data) == nil && data.Cron != "" {
			return data.Cron, true
		}
	}
	return "", false
}

func cronMatches(raw string, now time.Time) bool {
	parts := strings.Fields(raw)
	if len(parts) != 5 {
		return false
	}
	fields := [...]struct {
		value, min, max int
	}{
		{now.Minute(), 0, 59},
		{now.Hour(), 0, 23},
		{now.Day(), 1, 31},
		{int(now.Month()), 1, 12},
		{int(now.Weekday()), 0, 7},
	}
	for i := 0; i < 2; i++ {
		if !cronFieldMatches(parts[i], fields[i].value, fields[i].min, fields[i].max) {
			return false
		}
	}
	if !cronFieldMatches(parts[3], fields[3].value, fields[3].min, fields[3].max) {
		return false
	}

	dayOfMonth := cronFieldMatches(parts[2], fields[2].value, fields[2].min, fields[2].max)
	dayOfWeek := cronFieldMatches(parts[4], fields[4].value, fields[4].min, fields[4].max)
	if parts[2] == "*" || parts[4] == "*" {
		return dayOfMonth && dayOfWeek
	}
	return dayOfMonth || dayOfWeek
}

func cronFieldMatches(raw string, value, min, max int) bool {
	matched := false
	for _, term := range strings.Split(raw, ",") {
		base, stepText, hasStep := strings.Cut(term, "/")
		step := 1
		if hasStep {
			parsed, err := strconv.Atoi(stepText)
			if err != nil || parsed <= 0 {
				return false
			}
			step = parsed
		}

		low, high := min, max
		switch {
		case base == "*":
		case strings.Contains(base, "-"):
			bounds := strings.Split(base, "-")
			if len(bounds) != 2 {
				return false
			}
			var err error
			low, err = strconv.Atoi(bounds[0])
			if err != nil {
				return false
			}
			high, err = strconv.Atoi(bounds[1])
			if err != nil {
				return false
			}
		default:
			parsed, err := strconv.Atoi(base)
			if err != nil {
				return false
			}
			low, high = parsed, parsed
		}
		if low < min || high > max || low > high {
			return false
		}
		for _, candidate := range []int{value, 7} {
			if candidate == 7 && (max != 7 || value != 0) {
				continue
			}
			if candidate >= low && candidate <= high && (candidate-low)%step == 0 {
				matched = true
			}
		}
	}
	return matched
}
