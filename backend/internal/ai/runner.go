package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/R7rainz/switchyard/backend/internal/execution"
)

const (
	chatKind           = "chat"
	summarizeKind      = "summarize"
	classificationKind = "classification"
	decisionKind       = "decision"
)

// Runners is what this package contributes to the engine. The import points
// this way and never back: the engine knows the Runner interface and nothing
// about models, so a provider outage cannot be an execution package concern.
func Runners(service *Service) execution.Registry {
	return execution.Registry{
		"ai.prompt":         &promptRunner{ai: service},
		"ai.chat":           &promptRunner{ai: service, kind: chatKind},
		"ai.summarize":      &promptRunner{ai: service, kind: summarizeKind},
		"ai.classification": &promptRunner{ai: service, kind: classificationKind},
		"ai.decision":       &promptRunner{ai: service, kind: decisionKind},
	}
}

type promptRunner struct {
	ai   *Service
	kind string
}

func (p *promptRunner) Run(ctx context.Context, in execution.Input) (execution.Result, error) {
	var data struct {
		Provider     string          `json:"provider"`
		Prompt       string          `json:"prompt"`
		System       string          `json:"system"`
		Model        string          `json:"model"`
		Text         string          `json:"text"`
		Instructions string          `json:"instructions"`
		Question     string          `json:"question"`
		Context      string          `json:"context"`
		Labels       json.RawMessage `json:"labels"`
	}
	if len(in.Data) > 0 {
		if err := json.Unmarshal(in.Data, &data); err != nil {
			return execution.Result{}, fmt.Errorf("ai node data: %w", err)
		}
	}

	switch p.kind {
	case chatKind:
		return p.completeText(ctx, in, data.Provider, data.Prompt, data.System, data.Model)
	case summarizeKind:
		if strings.TrimSpace(data.Text) == "" {
			return execution.Result{}, fmt.Errorf("summarize node needs text")
		}
		prompt := "Summarize the following text clearly and concisely."
		if strings.TrimSpace(data.Instructions) != "" {
			prompt += " " + strings.TrimSpace(data.Instructions)
		}
		prompt += "\n\nText:\n" + data.Text
		return p.completeText(ctx, in, data.Provider, prompt, data.System, data.Model)
	case classificationKind:
		return p.classify(ctx, in, data.Provider, data.Text, data.Labels, data.Model)
	case decisionKind:
		return p.decide(ctx, in, data.Provider, data.Question, data.Context, data.System, data.Model)
	default:
		if strings.TrimSpace(data.Prompt) == "" {
			return execution.Result{}, fmt.Errorf("ai node needs a prompt")
		}
		return p.completeText(ctx, in, data.Provider, data.Prompt, data.System, data.Model)
	}
}

func (p *promptRunner) completeText(ctx context.Context, in execution.Input, provider, prompt, system, model string) (execution.Result, error) {
	if strings.TrimSpace(prompt) == "" {
		return execution.Result{}, fmt.Errorf("%s node needs a prompt", p.kindOrPrompt())
	}
	response, err := p.ai.Complete(ctx, in.WorkspaceID, Request{Provider: provider, Model: model, System: system, Prompt: prompt})
	if err != nil {
		return execution.Result{}, err
	}
	output, err := json.Marshal(map[string]any{"text": response.Text, "model": response.Model})
	if err != nil {
		return execution.Result{}, err
	}
	return execution.Result{Output: output}, nil
}

func (p *promptRunner) kindOrPrompt() string {
	if p.kind == "" {
		return "ai"
	}
	return p.kind
}

func (p *promptRunner) classify(ctx context.Context, in execution.Input, provider, text string, rawLabels json.RawMessage, model string) (execution.Result, error) {
	if strings.TrimSpace(text) == "" {
		return execution.Result{}, fmt.Errorf("classification node needs text")
	}
	labels, err := parseLabels(rawLabels)
	if err != nil {
		return execution.Result{}, err
	}
	prompt := fmt.Sprintf("Classify the following text using exactly one of these labels: %s. Return the best label and a short reason.\n\nText:\n%s", strings.Join(labels, ", "), text)
	response, err := p.ai.Complete(ctx, in.WorkspaceID, Request{
		Provider: provider,
		Model:    model,
		Prompt:   prompt,
		JSONSchema: &JSONSchema{
			Name:   "switchyard_classification",
			Schema: classificationSchema(labels),
			Strict: true,
		},
	})
	if err != nil {
		return execution.Result{}, err
	}
	var answer struct {
		Label     string `json:"label"`
		Reasoning string `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(stripFence(response.Text)), &answer); err != nil {
		return execution.Result{}, fmt.Errorf("classification node returned invalid JSON: %w", err)
	}
	if !contains(labels, answer.Label) {
		return execution.Result{}, fmt.Errorf("classification node returned unknown label %q", answer.Label)
	}
	output, err := json.Marshal(map[string]any{"label": answer.Label, "reasoning": answer.Reasoning, "model": response.Model})
	if err != nil {
		return execution.Result{}, err
	}
	return execution.Result{Output: output}, nil
}

func (p *promptRunner) decide(ctx context.Context, in execution.Input, provider, question, contextText, system, model string) (execution.Result, error) {
	if strings.TrimSpace(question) == "" {
		return execution.Result{}, fmt.Errorf("decision node needs a question")
	}
	prompt := "Answer the following decision with exactly true or false and explain briefly.\n\nQuestion:\n" + question
	if strings.TrimSpace(contextText) != "" {
		prompt += "\n\nContext:\n" + contextText
	}
	response, err := p.ai.Complete(ctx, in.WorkspaceID, Request{
		Provider: provider,
		Model:    model,
		System:   system,
		Prompt:   prompt,
		JSONSchema: &JSONSchema{
			Name:   "switchyard_decision",
			Schema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["decision","reasoning"],"properties":{"decision":{"type":"string","enum":["true","false"]},"reasoning":{"type":"string"}}}`),
			Strict: true,
		},
	})
	if err != nil {
		return execution.Result{}, err
	}
	var answer struct {
		Decision  string `json:"decision"`
		Reasoning string `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(stripFence(response.Text)), &answer); err != nil {
		return execution.Result{}, fmt.Errorf("decision node returned invalid JSON: %w", err)
	}
	if answer.Decision != "true" && answer.Decision != "false" {
		return execution.Result{}, fmt.Errorf("decision node returned %q, want true or false", answer.Decision)
	}
	output, err := json.Marshal(map[string]any{"decision": answer.Decision, "reasoning": answer.Reasoning, "model": response.Model})
	if err != nil {
		return execution.Result{}, err
	}
	return execution.Result{Output: output, Branch: answer.Decision}, nil
}

func parseLabels(raw json.RawMessage) ([]string, error) {
	raw = unwrapJSONText(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("classification node needs labels")
	}
	var labels []string
	if err := json.Unmarshal(raw, &labels); err != nil {
		return nil, fmt.Errorf("classification node labels must be a JSON array of strings: %w", err)
	}
	if len(labels) < 2 {
		return nil, fmt.Errorf("classification node needs at least two labels")
	}
	seen := make(map[string]struct{}, len(labels))
	for i, label := range labels {
		labels[i] = strings.TrimSpace(label)
		if labels[i] == "" {
			return nil, fmt.Errorf("classification node labels cannot be empty")
		}
		if _, duplicate := seen[labels[i]]; duplicate {
			return nil, fmt.Errorf("classification node label %q is duplicated", labels[i])
		}
		seen[labels[i]] = struct{}{}
	}
	return labels, nil
}

func unwrapJSONText(raw json.RawMessage) json.RawMessage {
	var text string
	if len(raw) > 0 && json.Unmarshal(raw, &text) == nil {
		return json.RawMessage(text)
	}
	return raw
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func classificationSchema(labels []string) json.RawMessage {
	encoded, _ := json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"label", "reasoning"},
		"properties": map[string]any{
			"label":     map[string]any{"type": "string", "enum": labels},
			"reasoning": map[string]any{"type": "string"},
		},
	})
	return encoded
}
