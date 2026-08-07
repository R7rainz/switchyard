package github

import (
	"bytes"
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

type githubAction struct {
	credentials Credentials
	client      *http.Client
	baseURL     string
}

type issueRunner struct{ githubAction }
type commentRunner struct{ githubAction }
type mergeRunner struct{ githubAction }

func (g githubAction) token(ctx context.Context, workspaceID string) (string, error) {
	token, err := g.credentials.Get(ctx, workspaceID, credentialProvider, credentialName)
	if errors.Is(err, credential.ErrNotFound) {
		return "", ErrNoCredential
	}
	if err != nil {
		return "", fmt.Errorf("github credential: %w", err)
	}
	return string(token), nil
}

func (g githubAction) request(ctx context.Context, token, method, path string, body any, output any) error {
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(g.baseURL, "/")+path, payload)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := g.client.Do(request)
	if err != nil {
		return fmt.Errorf("github request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("github request: %s", response.Status)
	}
	if output != nil {
		if err := json.NewDecoder(response.Body).Decode(output); err != nil {
			return fmt.Errorf("github response: %w", err)
		}
	}
	return nil
}

func actionData(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func field(data map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(data[key], &value)
	return strings.TrimSpace(value)
}

func actionRepo(data map[string]json.RawMessage) (owner, repo string, number string, err error) {
	owner, repo = field(data, "owner"), field(data, "repo")
	if owner == "" || repo == "" {
		return "", "", "", errors.New("github node needs owner and repo")
	}
	if len(data["number"]) > 0 {
		number, err = numberString(data["number"])
		if err != nil {
			return "", "", "", errors.New("github node needs a valid number")
		}
	}
	return owner, repo, number, nil
}

func (g *issueRunner) Run(ctx context.Context, in execution.Input) (execution.Result, error) {
	data, err := actionData(in.Data)
	if err != nil {
		return execution.Result{}, fmt.Errorf("github issue data: %w", err)
	}
	owner, repo, _, err := actionRepo(data)
	if err != nil {
		return execution.Result{}, err
	}
	title := field(data, "title")
	if title == "" {
		return execution.Result{}, errors.New("github issue needs title")
	}
	token, err := g.token(ctx, in.WorkspaceID)
	if err != nil {
		return execution.Result{}, err
	}
	var created map[string]any
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/issues"
	body := map[string]string{"title": title, "body": field(data, "body")}
	if err := g.request(ctx, token, http.MethodPost, path, body, &created); err != nil {
		return execution.Result{}, fmt.Errorf("github issue: %w", err)
	}
	output, _ := json.Marshal(map[string]any{
		"number": created["number"], "title": created["title"], "url": created["html_url"],
	})
	return execution.Result{Output: output}, nil
}

func (g *commentRunner) Run(ctx context.Context, in execution.Input) (execution.Result, error) {
	data, err := actionData(in.Data)
	if err != nil {
		return execution.Result{}, fmt.Errorf("github comment data: %w", err)
	}
	owner, repo, number, err := actionRepo(data)
	if err != nil || number == "" {
		return execution.Result{}, errors.New("github comment needs owner, repo, and number")
	}
	body := field(data, "body")
	if body == "" {
		return execution.Result{}, errors.New("github comment needs body")
	}
	token, err := g.token(ctx, in.WorkspaceID)
	if err != nil {
		return execution.Result{}, err
	}
	var comment map[string]any
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/issues/" + url.PathEscape(number) + "/comments"
	if err := g.request(ctx, token, http.MethodPost, path, map[string]string{"body": body}, &comment); err != nil {
		return execution.Result{}, fmt.Errorf("github comment: %w", err)
	}
	output, _ := json.Marshal(map[string]any{"id": comment["id"], "url": comment["html_url"]})
	return execution.Result{Output: output}, nil
}

func (g *mergeRunner) Run(ctx context.Context, in execution.Input) (execution.Result, error) {
	data, err := actionData(in.Data)
	if err != nil {
		return execution.Result{}, fmt.Errorf("github merge data: %w", err)
	}
	owner, repo, number, err := actionRepo(data)
	if err != nil || number == "" {
		return execution.Result{}, errors.New("github merge needs owner, repo, and number")
	}
	method := field(data, "method")
	if method == "" {
		method = "merge"
	}
	if method != "merge" && method != "squash" && method != "rebase" {
		return execution.Result{}, errors.New("github merge method must be merge, squash, or rebase")
	}
	token, err := g.token(ctx, in.WorkspaceID)
	if err != nil {
		return execution.Result{}, err
	}
	var merged map[string]any
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/pulls/" + url.PathEscape(number) + "/merge"
	if err := g.request(ctx, token, http.MethodPut, path, map[string]string{"merge_method": method}, &merged); err != nil {
		return execution.Result{}, fmt.Errorf("github merge: %w", err)
	}
	output, _ := json.Marshal(map[string]any{
		"merged": merged["merged"], "message": merged["message"], "sha": merged["sha"],
	})
	return execution.Result{Output: output}, nil
}
