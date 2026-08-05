package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestPullRequestRunnerReadsOnlyTheFieldsAWorkflowNeeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets/pulls/42" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("request = %s, auth = %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"number":42,"title":"Ship it","body":"Details","html_url":"https://github.com/acme/widgets/pull/42","user":{"login":"dev"},"base":{"ref":"main"},"head":{"ref":"feature"}}`))
	}))
	defer server.Close()

	runner := Runners(testCredentials{"github/default": credential.Secret("token")}, server.Client())["github.pull_request"].(*pullRequestRunner)
	runner.baseURL = server.URL
	result, err := runner.Run(t.Context(), execution.Input{WorkspaceID: "ws", Data: []byte(`{"owner":"acme","repo":"widgets","number":"42"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatal(err)
	}
	if output["title"] != "Ship it" || output["author"] != "dev" || output["base"] != "main" {
		t.Fatalf("output = %v", output)
	}
}
