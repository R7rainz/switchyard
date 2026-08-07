package ai

import (
	"context"
	"net/http"
)

// OpenAI uses the OpenAI chat-completions wire format directly. OpenRouter is
// intentionally a separate adapter because its routing options are not part
// of OpenAI's API.
type OpenAI struct {
	client  *http.Client
	baseURL string
}

func NewOpenAI(client *http.Client) *OpenAI {
	return &OpenAI{client: providerClient(client), baseURL: "https://api.openai.com/v1"}
}

func (o *OpenAI) Complete(ctx context.Context, key string, req Request) (Response, error) {
	// OpenAI's strict structured-output subset requires every object in the
	// schema to close additional properties. Workflow node data is deliberately
	// extensible because each node owns its configuration, so keep the schema
	// guidance but use OpenAI's non-strict JSON Schema mode for workflow drafts.
	// The smaller node result schemas remain strict. parseGenerated still
	// validates the returned graph before it reaches the caller.
	if req.JSONSchema != nil && req.JSONSchema.Name == "switchyard_workflow" {
		schema := *req.JSONSchema
		schema.Strict = false
		req.JSONSchema = &schema
	}
	return completeChatCompletions(ctx, o.client, o.baseURL+"/chat/completions", map[string]string{
		"Authorization": "Bearer " + key,
	}, req, false, "max_completion_tokens")
}
