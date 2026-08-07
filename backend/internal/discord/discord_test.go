package discord

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

func TestMessageRunnerPostsDiscordPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "{\"content\":\"hello\"}" {
			t.Fatalf("body = %s", body)
		}
	}))
	defer server.Close()
	runner := Runners(testCredentials{"discord/default": credential.Secret("https://discord.com/api/webhooks/test")}, server.Client())["discord.message"].(*messageRunner)
	runner.allowed = func(*url.URL) bool { return true }
	runner.credentials = testCredentials{"discord/default": credential.Secret(server.URL)}
	result, err := runner.Run(t.Context(), execution.Input{WorkspaceID: "ws", Data: []byte("{\"text\":\"hello\"}")})
	if err != nil || string(result.Output) != "{\"sent\":true}" {
		t.Fatalf("result = %s, err = %v", result.Output, err)
	}
}
