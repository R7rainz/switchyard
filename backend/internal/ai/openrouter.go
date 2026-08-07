package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenRouter talks to https://openrouter.ai, which speaks the OpenAI chat
// completions shape. That is the reason it is the default: one key, one wire
// format, and every model behind it.
type OpenRouter struct {
	client  *http.Client
	baseURL string
}

// openRouterBackstop is a last resort, not the real bound.
//
// Each caller sets its own deadline on the context, because they are not the
// same: generation answers an HTTP request and has to fit under the server's
// write timeout, while an ai.prompt node has the engine's per-node budget and
// nobody waiting on a connection. A client timeout short enough for the first
// would silently cut the second short — the two mechanisms bounding one call
// is how that happens without anyone noticing.
//
// This exists only so a caller that forgets a deadline cannot hang forever.
const openRouterBackstop = 2 * time.Minute

func NewOpenRouter(client *http.Client) *OpenRouter {
	if client == nil {
		client = &http.Client{Timeout: openRouterBackstop}
	}
	return &OpenRouter{client: client, baseURL: "https://openrouter.ai/api/v1"}
}

// maxProviderResponse caps what one completion brings back. The text is stored
// on a node row and streamed to a browser.
const maxProviderResponse = 1 << 20

func (o *OpenRouter) Complete(ctx context.Context, key string, req Request) (Response, error) {
	messages := make([]map[string]string, 0, 2)
	if req.System != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.System})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.Prompt})

	body := map[string]any{"model": req.Model, "messages": messages}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.JSONSchema != nil {
		if req.JSONSchema.Name == "" || len(req.JSONSchema.Schema) == 0 {
			return Response{}, fmt.Errorf("%w: JSON schema needs a name and schema", ErrProvider)
		}
		schema := *req.JSONSchema
		// Workflow node data is intentionally open-ended. OpenRouter forwards
		// strict schemas to routed providers that reject an object unless every
		// object declares additionalProperties=false, so use JSON-schema guidance
		// without strict mode for the extensible workflow envelope.
		if schema.Name == "switchyard_workflow" {
			schema.Strict = false
		}
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   schema.Name,
				"strict": schema.Strict,
				"schema": json.RawMessage(schema.Schema),
			},
		}
		// Do not silently route to a provider that drops structured-output
		// parameters; generation must either honor the schema or fail clearly.
		body["provider"] = map[string]any{"require_parameters": true}
	} else if req.JSON {
		body["response_format"] = map[string]string{"type": "json_object"}
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(encoded))
	if err != nil {
		return Response{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)

	response, err := o.client.Do(request)
	if err != nil {
		// The URL is in here but never the key: it travels in a header, and
		// Go's own transport errors do not quote headers.
		return Response{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponse))
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}

	var decoded struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	// A non-2xx usually still carries the JSON error body, so decode before
	// judging the status: "insufficient credits" is worth passing on, and
	// "502 Bad Gateway" alone is not.
	_ = json.Unmarshal(raw, &decoded)

	if response.StatusCode >= 400 {
		if decoded.Error.Message != "" {
			return Response{}, fmt.Errorf("%w: %s (%s)", ErrProvider, decoded.Error.Message, response.Status)
		}
		return Response{}, fmt.Errorf("%w: %s", ErrProvider, response.Status)
	}
	if len(decoded.Choices) == 0 {
		return Response{}, fmt.Errorf("%w: the model returned no choices", ErrProvider)
	}

	return Response{Text: decoded.Choices[0].Message.Content, Model: decoded.Model}, nil
}
