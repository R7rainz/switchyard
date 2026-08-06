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
	return completeChatCompletions(ctx, o.client, o.baseURL+"/chat/completions", map[string]string{
		"Authorization": "Bearer " + key,
	}, req, false, "max_completion_tokens")
}
