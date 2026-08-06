package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Anthropic struct {
	client  *http.Client
	baseURL string
}

func NewAnthropic(client *http.Client) *Anthropic {
	return &Anthropic{client: providerClient(client), baseURL: "https://api.anthropic.com/v1"}
}

func (a *Anthropic) Complete(ctx context.Context, key string, req Request) (Response, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	body := map[string]any{
		"model":      req.Model,
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": jsonPrompt(req)},
		},
	}
	if req.System != "" {
		body["system"] = req.System
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/messages", bytes.NewReader(encoded))
	if err != nil {
		return Response{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", key)
	request.Header.Set("anthropic-version", "2023-06-01")

	response, err := a.client.Do(request)
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
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
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
	if len(decoded.Content) == 0 || decoded.Content[0].Text == "" {
		return Response{}, fmt.Errorf("%w: the model returned no content", ErrProvider)
	}
	return Response{Text: decoded.Content[0].Text, Model: decoded.Model}, nil
}
