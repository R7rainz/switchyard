package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func completeChatCompletions(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, req Request, requireParameters bool, tokenField string) (Response, error) {
	messages := make([]map[string]string, 0, 2)
	if req.System != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.System})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.Prompt})

	body := map[string]any{"model": req.Model, "messages": messages}
	if req.MaxTokens > 0 {
		if tokenField == "" {
			tokenField = "max_tokens"
		}
		body[tokenField] = req.MaxTokens
	}
	if req.JSONSchema != nil {
		if req.JSONSchema.Name == "" || len(req.JSONSchema.Schema) == 0 {
			return Response{}, fmt.Errorf("%w: JSON schema needs a name and schema", ErrProvider)
		}
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   req.JSONSchema.Name,
				"strict": req.JSONSchema.Strict,
				"schema": json.RawMessage(req.JSONSchema.Schema),
			},
		}
		if requireParameters {
			body["provider"] = map[string]any{"require_parameters": true}
		}
	} else if req.JSON {
		body["response_format"] = map[string]string{"type": "json_object"}
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return Response{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := client.Do(request)
	if err != nil {
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

func jsonPrompt(req Request) string {
	if req.JSONSchema == nil && !req.JSON {
		return req.Prompt
	}
	prompt := req.Prompt + "\n\nReturn only valid JSON, with no markdown or explanation."
	if req.JSONSchema != nil && len(req.JSONSchema.Schema) > 0 {
		prompt += " The JSON must match this schema:\n" + string(req.JSONSchema.Schema)
	}
	return prompt
}

func providerClient(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{Timeout: openRouterBackstop}
	}
	return client
}

func trimModelPrefix(model string) string {
	return strings.TrimPrefix(model, "models/")
}
