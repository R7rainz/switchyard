package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenRouterSendsJSONSchema(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer server.Close()

	client := &OpenRouter{client: server.Client(), baseURL: server.URL}
	_, err := client.Complete(t.Context(), "sk-test", Request{
		Model: "m", Prompt: "hi",
		JSONSchema: &JSONSchema{Name: "thing", Schema: json.RawMessage(`{"type":"object"}`), Strict: true},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	format, ok := got["response_format"].(map[string]any)
	if !ok || format["type"] != "json_schema" {
		t.Fatalf("response_format = %#v", got["response_format"])
	}
	schema, ok := format["json_schema"].(map[string]any)
	if !ok || schema["name"] != "thing" || schema["strict"] != true {
		t.Fatalf("json_schema = %#v", format["json_schema"])
	}
	provider, ok := got["provider"].(map[string]any)
	if !ok || provider["require_parameters"] != true {
		t.Fatalf("provider = %#v, want require_parameters", got["provider"])
	}
}

// The provider's own error message is passed on — "insufficient credits" is
// something the caller can act on, and a bare 402 is not.
func TestOpenRouterSurfacesTheProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"message":"insufficient credits"}}`))
	}))
	defer server.Close()

	client := &OpenRouter{client: server.Client(), baseURL: server.URL}
	_, err := client.Complete(t.Context(), "sk-test", Request{Model: "m", Prompt: "hi"})

	if !errors.Is(err, ErrProvider) {
		t.Fatalf("err = %v, want ErrProvider", err)
	}
	if !strings.Contains(err.Error(), "insufficient credits") {
		t.Fatalf("the provider's reason was dropped: %v", err)
	}
	// The key must never reach an error string: these travel into logs and
	// into HTTP responses.
	if strings.Contains(err.Error(), "sk-test") {
		t.Fatalf("the API key leaked into an error: %v", err)
	}
}

func TestOpenRouterSendsTheKeyAndReturnsTheText(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"hello"}}]}`))
	}))
	defer server.Close()

	client := &OpenRouter{client: server.Client(), baseURL: server.URL}
	response, err := client.Complete(t.Context(), "sk-test", Request{Model: "m", Prompt: "hi"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if response.Text != "hello" {
		t.Fatalf("text = %q", response.Text)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}

// The client must not carry a deadline of its own that is shorter than what a
// caller asked for. Generation needs ~25s because it answers an HTTP request;
// an ai.prompt node has the engine's per-node budget and nobody waiting on a
// connection, and a client timeout tuned for the first would cut the second
// short with nothing in the code saying so.
func TestClientTimeoutDoesNotUndercutTheCaller(t *testing.T) {
	client := NewOpenRouter(nil).client
	if client.Timeout < time.Minute {
		t.Fatalf("client timeout %s is short enough to pre-empt a caller's own deadline", client.Timeout)
	}
}

// The caller's deadline is what actually stops a slow model.
func TestTheCallersDeadlineIsHonoured(t *testing.T) {
	// A bounded sleep rather than blocking on the request context: httptest's
	// Close waits for handlers to return, and an HTTP/1.1 server does not
	// notice a client hanging up until it writes.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"too late"}}]}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	client := &OpenRouter{client: server.Client(), baseURL: server.URL}
	started := time.Now()
	if _, err := client.Complete(ctx, "sk-test", Request{Model: "m", Prompt: "hi"}); err == nil {
		t.Fatal("a call past its deadline must fail")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("the deadline was ignored: took %s", elapsed)
	}
}
