package slack

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/execution"
)

type testCredentials map[string]credential.Secret

func (c testCredentials) Get(_ context.Context, _ string, provider, name string) (credential.Secret, error) {
	if secret, ok := c[provider+"/"+name]; ok {
		return secret, nil
	}
	return nil, credential.ErrNotFound
}

func TestMessageRunnerPostsTheRenderedText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || string(body) != `{"text":"PR #42 is ready"}` {
			t.Fatalf("method = %s, body = %s", r.Method, body)
		}
	}))
	defer server.Close()

	runner := Runners(testCredentials{"slack/default": credential.Secret(server.URL)}, server.Client())["slack.message"].(*messageRunner)
	runner.allowed = func(*url.URL) bool { return true }
	result, err := runner.Run(t.Context(), execution.Input{WorkspaceID: "ws", Data: []byte(`{"text":"PR #42 is ready"}`)})
	if err != nil || string(result.Output) != `{"sent":true}` {
		t.Fatalf("result = %s, err = %v", result.Output, err)
	}
}
