package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestActionRunnersSendExpectedGitHubRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widgets/issues":
			if r.Method != http.MethodPost {
				t.Fatalf("issue method = %s", r.Method)
			}
			_, _ = w.Write([]byte(`{"number":7,"title":"Bug","html_url":"https://github.com/acme/widgets/issues/7"}`))
		case "/repos/acme/widgets/issues/7/comments":
			_, _ = w.Write([]byte(`{"id":8,"html_url":"https://github.com/acme/widgets/issues/7#issuecomment-8"}`))
		case "/repos/acme/widgets/pulls/7/merge":
			_, _ = w.Write([]byte(`{"merged":true,"message":"merged","sha":"abc"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	credentials := testCredentials{"github/default": credential.Secret("token")}
	cases := []struct {
		name, node string
		data       string
		want       string
	}{
		{"issue", "github.issue", `{"owner":"acme","repo":"widgets","title":"Bug"}`, "7"},
		{"comment", "github.comment", `{"owner":"acme","repo":"widgets","number":7,"body":"Fixed"}`, "8"},
		{"merge", "github.merge", `{"owner":"acme","repo":"widgets","number":7}`, "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := Runners(credentials, server.Client())[tc.node]
			switch typed := runner.(type) {
			case *issueRunner:
				typed.baseURL = server.URL
			case *commentRunner:
				typed.baseURL = server.URL
			case *mergeRunner:
				typed.baseURL = server.URL
			}
			result, err := runner.Run(t.Context(), execution.Input{WorkspaceID: "ws", Data: []byte(tc.data)})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(result.Output), tc.want) {
				t.Fatalf("output = %s, want %q", result.Output, tc.want)
			}
		})
	}
}
