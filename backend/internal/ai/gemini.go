package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Gemini struct {
	client  *http.Client
	baseURL string
}

func NewGemini(client *http.Client) *Gemini {
	return &Gemini{client: providerClient(client), baseURL: "https://generativelanguage.googleapis.com/v1beta"}
}

func (g *Gemini) Complete(ctx context.Context, key string, req Request) (Response, error) {
	model := trimModelPrefix(req.Model)
	body := map[string]any{
		"contents": []map[string]any{{
			"role":  "user",
			"parts": []map[string]string{{"text": jsonPrompt(req)}},
		}},
	}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{"parts": []map[string]string{{"text": req.System}}}
	}
	config := map[string]any{}
	if req.MaxTokens > 0 {
		config["maxOutputTokens"] = req.MaxTokens
	}
	if req.JSON || req.JSONSchema != nil {
		config["responseMimeType"] = "application/json"
	}
	if len(config) > 0 {
		body["generationConfig"] = config
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	endpoint := g.baseURL + "/models/" + url.PathEscape(model) + ":generateContent"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return Response{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-goog-api-key", key)

	response, err := g.client.Do(request)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponse))
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	var decoded struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
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
	if len(decoded.Candidates) == 0 || len(decoded.Candidates[0].Content.Parts) == 0 {
		return Response{}, fmt.Errorf("%w: the model returned no content", ErrProvider)
	}
	return Response{Text: decoded.Candidates[0].Content.Parts[0].Text, Model: req.Model}, nil
}
