package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/credential"
)

func TestOpenAIUsesChatCompletions(t *testing.T) {
	var body map[string]any
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte("{\"model\":\"gpt-test\",\"choices\":[{\"message\":{\"content\":\"hello\"}}]}"))
	}))
	defer server.Close()

	client := &OpenAI{client: server.Client(), baseURL: server.URL}
	response, err := client.Complete(t.Context(), "sk-openai", Request{
		Model: "gpt-test", Prompt: "hi", MaxTokens: 32, JSON: true,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if response.Text != "hello" || auth != "Bearer sk-openai" {
		t.Fatalf("response = %+v, auth = %q", response, auth)
	}
	if body["response_format"].(map[string]any)["type"] != "json_object" {
		t.Fatalf("request did not ask for JSON: %#v", body)
	}
	if body["max_completion_tokens"] != float64(32) {
		t.Fatalf("OpenAI request used the wrong token field: %#v", body)
	}
}

func TestOpenAIAcceptsExtensibleWorkflowSchema(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte("{\"model\":\"gpt-test\",\"choices\":[{\"message\":{\"content\":\"{}\"}}]}"))
	}))
	defer server.Close()

	client := &OpenAI{client: server.Client(), baseURL: server.URL}
	_, err := client.Complete(t.Context(), "sk-openai", Request{
		Model: "gpt-test", Prompt: "hi",
		JSONSchema: &JSONSchema{
			Name:   "switchyard_workflow",
			Schema: json.RawMessage(`{"type":"object","properties":{"data":{"type":"object","additionalProperties":true}}}`),
			Strict: true,
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	format := body["response_format"].(map[string]any)
	schema := format["json_schema"].(map[string]any)
	if schema["strict"] != false {
		t.Fatalf("json_schema strict = %#v, want false", schema["strict"])
	}
}

func TestAnthropicUsesMessagesAPI(t *testing.T) {
	var body map[string]any
	var gotKey, version string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		version = r.Header.Get("anthropic-version")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte("{\"model\":\"claude-test\",\"content\":[{\"type\":\"text\",\"text\":\"hello\"}]}"))
	}))
	defer server.Close()

	client := &Anthropic{client: server.Client(), baseURL: server.URL}
	response, err := client.Complete(t.Context(), "sk-anthropic", Request{Model: "claude-test", Prompt: "hi"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if response.Text != "hello" || gotKey != "sk-anthropic" || version == "" {
		t.Fatalf("response = %+v, key = %q, version = %q", response, gotKey, version)
	}
	if body["max_tokens"] == nil {
		t.Fatalf("Anthropic request omitted required max_tokens: %#v", body)
	}
}

func TestGeminiUsesGenerateContent(t *testing.T) {
	var body map[string]any
	var gotKey, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-goog-api-key")
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte("{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]}}]}"))
	}))
	defer server.Close()

	client := &Gemini{client: server.Client(), baseURL: server.URL}
	response, err := client.Complete(t.Context(), "gemini-key", Request{Model: "gemini-test", Prompt: "hi", JSON: true})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if response.Text != "hello" || gotKey != "gemini-key" || !strings.HasSuffix(path, "/models/gemini-test:generateContent") {
		t.Fatalf("response = %+v, key = %q, path = %q", response, gotKey, path)
	}
	config := body["generationConfig"].(map[string]any)
	if config["responseMimeType"] != "application/json" {
		t.Fatalf("Gemini request did not ask for JSON: %#v", body)
	}
}

func TestServiceSelectsProviderAndCredential(t *testing.T) {
	provider := &stubProvider{replies: []string{"ok"}}
	creds := providerCreds{keys: map[string]string{ProviderOpenAI: "openai-key"}}
	service := NewServiceWithProviders(map[string]Provider{ProviderOpenAI: provider}, creds)

	response, err := service.Complete(t.Context(), "ws", Request{Provider: ProviderOpenAI, Prompt: "hi"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if response.Text != "ok" || len(provider.keys) != 1 || provider.keys[0] != "openai-key" {
		t.Fatalf("response = %+v, keys = %#v", response, provider.keys)
	}
	if provider.requests[0].Model != DefaultModelFor(ProviderOpenAI) {
		t.Fatalf("model = %q", provider.requests[0].Model)
	}
}

type providerCreds struct{ keys map[string]string }

func (c providerCreds) Get(_ context.Context, _ string, provider, _ string) (credential.Secret, error) {
	key, ok := c.keys[provider]
	if !ok {
		return nil, credential.ErrNotFound
	}
	return credential.Secret(key), nil
}
