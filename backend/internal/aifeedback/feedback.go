// Package aifeedback stores workflow-generation feedback only after an
// explicit user opt-in.
package aifeedback

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

var ErrInvalid = errors.New("ai feedback: invalid submission")

type Outcome string

const (
	OutcomeAccepted Outcome = "accepted"
	OutcomeRejected Outcome = "rejected"
)

// Submission is the data a user explicitly permits Switchyard to retain for
// improving workflow generation. It is never created by GenerateWorkflow.
type Submission struct {
	WorkspaceID          string
	UserID               string
	Prompt               string
	Outcome              Outcome
	GeneratedName        string
	GeneratedDescription string
	GeneratedGraph       workflow.Graph
	FinalGraph           *workflow.Graph
}

// Record is the redacted form written to storage.
type Record struct {
	ID                   string
	WorkspaceID          string
	UserID               string
	Prompt               string
	Outcome              Outcome
	GeneratedName        string
	GeneratedDescription string
	GeneratedGraph       json.RawMessage
	FinalGraph           json.RawMessage
	CreatedAt            time.Time
}

type Store interface {
	Create(ctx context.Context, record Record) error
}

type Service struct {
	store Store
	now   func() time.Time
	newID func() string
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now, newID: randomID}
}

const (
	maxPromptBytes       = 16 << 10
	maxNameBytes         = 200
	maxDescriptionBytes  = 2000
	maxRedactedGraphSize = 2 << 20
)

// Submit validates, redacts, and stores one opted-in feedback record.
func (s *Service) Submit(ctx context.Context, input Submission) error {
	if input.WorkspaceID == "" || input.UserID == "" {
		return fmt.Errorf("%w: workspace and user are required", ErrInvalid)
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return fmt.Errorf("%w: prompt is required", ErrInvalid)
	}
	if len(input.Prompt) > maxPromptBytes {
		return fmt.Errorf("%w: prompt is limited to %d bytes", ErrInvalid, maxPromptBytes)
	}
	if len(input.GeneratedName) > maxNameBytes {
		return fmt.Errorf("%w: generated name is limited to %d bytes", ErrInvalid, maxNameBytes)
	}
	if len(input.GeneratedDescription) > maxDescriptionBytes {
		return fmt.Errorf("%w: generated description is limited to %d bytes", ErrInvalid, maxDescriptionBytes)
	}
	if input.Outcome != OutcomeAccepted && input.Outcome != OutcomeRejected {
		return fmt.Errorf("%w: outcome must be accepted or rejected", ErrInvalid)
	}
	if err := input.GeneratedGraph.Validate(); err != nil {
		return fmt.Errorf("%w: generated graph: %v", ErrInvalid, err)
	}
	if input.FinalGraph != nil {
		if err := input.FinalGraph.Validate(); err != nil {
			return fmt.Errorf("%w: final graph: %v", ErrInvalid, err)
		}
	}

	generated, err := redactGraph(input.GeneratedGraph)
	if err != nil {
		return fmt.Errorf("%w: generated graph: %v", ErrInvalid, err)
	}
	var final json.RawMessage
	if input.FinalGraph != nil {
		final, err = redactGraph(*input.FinalGraph)
		if err != nil {
			return fmt.Errorf("%w: final graph: %v", ErrInvalid, err)
		}
	}

	record := Record{
		ID:                   s.newID(),
		WorkspaceID:          input.WorkspaceID,
		UserID:               input.UserID,
		Prompt:               redactText(strings.TrimSpace(input.Prompt)),
		Outcome:              input.Outcome,
		GeneratedName:        redactText(strings.TrimSpace(input.GeneratedName)),
		GeneratedDescription: redactText(strings.TrimSpace(input.GeneratedDescription)),
		GeneratedGraph:       generated,
		FinalGraph:           final,
		CreatedAt:            s.now(),
	}
	if err := s.store.Create(ctx, record); err != nil {
		return err
	}
	return nil
}

func randomID() string { return rand.Text() }

var (
	secretKey = regexp.MustCompile(`(?i)^(authorization|password|secret|token|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|private[_-]?key|webhook)$`)
	bearer    = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	knownKey  = regexp.MustCompile(`\b(?:sk|gh[pousr]|xox[baprs])-[-A-Za-z0-9_]{8,}\b`)
)

// redactText covers common credential forms in free-form prompts. Structured
// graph fields use redactValue below, where key names give us a safer boundary.
func redactText(text string) string {
	text = bearer.ReplaceAllString(text, "Bearer [REDACTED]")
	return knownKey.ReplaceAllString(text, "[REDACTED]")
}

func redactGraph(graph workflow.Graph) (json.RawMessage, error) {
	raw, err := json.Marshal(graph)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	redacted := redactValue(value)
	raw, err = json.Marshal(redacted)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxRedactedGraphSize {
		return nil, fmt.Errorf("graph is limited to %d bytes", maxRedactedGraphSize)
	}
	return raw, nil
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if secretKey.MatchString(key) {
				typed[key] = "[REDACTED]"
				continue
			}
			typed[key] = redactValue(child)
		}
	case []any:
		for i := range typed {
			typed[i] = redactValue(typed[i])
		}
	case string:
		return redactText(typed)
	}
	return value
}
