package ai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
