package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/R7rainz/switchyard/backend/internal/execution"
)

// Runners is what this package contributes to the engine. The import points
// this way and never back: the engine knows the Runner interface and nothing
// about models, so a provider outage cannot be an execution package concern.
func Runners(service *Service) execution.Registry {
	return execution.Registry{"ai.prompt": &promptRunner{ai: service}}
}

type promptRunner struct {
	ai *Service
}

func (p *promptRunner) Run(ctx context.Context, in execution.Input) (execution.Result, error) {
	var data struct {
		Prompt string `json:"prompt"`
		System string `json:"system"`
		Model  string `json:"model"`
	}
	if len(in.Data) > 0 {
		if err := json.Unmarshal(in.Data, &data); err != nil {
			return execution.Result{}, fmt.Errorf("ai node data: %w", err)
		}
	}
	if data.Prompt == "" {
		return execution.Result{}, fmt.Errorf("ai node needs a prompt")
	}

	// The workspace's key, looked up here rather than handed to the engine, so
	// no secret sits in a struct the run logs or stores.
	response, err := p.ai.Complete(ctx, in.WorkspaceID, Request{
		Model:  data.Model,
		System: data.System,
		Prompt: data.Prompt,
	})
	if err != nil {
		return execution.Result{}, err
	}

	output, err := json.Marshal(map[string]any{"text": response.Text, "model": response.Model})
	if err != nil {
		return execution.Result{}, err
	}
	return execution.Result{Output: output}, nil
}
