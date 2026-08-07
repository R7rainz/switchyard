package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/execution"
)

const credentialProvider = "github"
const credentialName = "default"

var ErrNoCredential = errors.New("github: missing credential github/default")

// Credentials is the one lookup this integration needs. The credential service
// remains the only place plaintext tokens can be opened.
type Credentials interface {
	Get(context.Context, string, string, string) (credential.Secret, error)
}

// Runners exposes GitHub's workflow nodes to the execution engine.
func Runners(credentials Credentials, client *http.Client) execution.Registry {
	if client == nil {
		client = http.DefaultClient
	}
	baseURL := "https://api.github.com"
	return execution.Registry{
		"github.pull_request": &pullRequestRunner{credentials: credentials, client: client, baseURL: baseURL},
		"github.issue":        &issueRunner{githubAction{credentials: credentials, client: client, baseURL: baseURL}},
		"github.comment":      &commentRunner{githubAction{credentials: credentials, client: client, baseURL: baseURL}},
		"github.merge":        &mergeRunner{githubAction{credentials: credentials, client: client, baseURL: baseURL}},
	}
}

type pullRequestRunner struct {
	credentials Credentials
	client      *http.Client
	baseURL     string
}

func (g *pullRequestRunner) Run(ctx context.Context, in execution.Input) (execution.Result, error) {
	var data struct {
		Owner  string          `json:"owner"`
		Repo   string          `json:"repo"`
		Number json.RawMessage `json:"number"`
	}
	if err := json.Unmarshal(in.Data, &data); err != nil {
		return execution.Result{}, fmt.Errorf("github pull request data: %w", err)
	}
	number, err := numberString(data.Number)
	if err != nil || data.Owner == "" || data.Repo == "" {
		return execution.Result{}, errors.New("github pull request needs owner, repo, and number")
	}

	token, err := g.credentials.Get(ctx, in.WorkspaceID, credentialProvider, credentialName)
	if errors.Is(err, credential.ErrNotFound) {
		return execution.Result{}, ErrNoCredential
	}
	if err != nil {
		return execution.Result{}, fmt.Errorf("github credential: %w", err)
	}

	endpoint := strings.TrimRight(g.baseURL, "/") + "/repos/" + url.PathEscape(data.Owner) + "/" + url.PathEscape(data.Repo) + "/pulls/" + url.PathEscape(number)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return execution.Result{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+string(token))
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	response, err := g.client.Do(request)
	if err != nil {
		return execution.Result{}, fmt.Errorf("github pull request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return execution.Result{}, fmt.Errorf("github pull request: %s", response.Status)
	}

	var pullRequest struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
	}
	if err := json.NewDecoder(response.Body).Decode(&pullRequest); err != nil {
		return execution.Result{}, fmt.Errorf("github pull request response: %w", err)
	}

	output, err := json.Marshal(map[string]any{
		"number": pullRequest.Number, "title": pullRequest.Title, "body": pullRequest.Body,
		"url": pullRequest.HTMLURL, "author": pullRequest.User.Login,
		"base": pullRequest.Base.Ref, "head": pullRequest.Head.Ref,
	})
	if err != nil {
		return execution.Result{}, err
	}
	return execution.Result{Output: output}, nil
}

func numberString(raw json.RawMessage) (string, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil && text != "" {
		return text, nil
	}
	var number json.Number
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil || number.String() == "" {
		return "", errors.New("missing number")
	}
	return number.String(), nil
}
