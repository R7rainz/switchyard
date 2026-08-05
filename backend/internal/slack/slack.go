package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/execution"
)

const credentialProvider = "slack"
const credentialName = "default"

var ErrNoCredential = errors.New("slack: missing credential slack/default")

type Credentials interface {
	Get(context.Context, string, string, string) (credential.Secret, error)
}

func Runners(credentials Credentials, client *http.Client) execution.Registry {
	if client == nil {
		client = http.DefaultClient
	}
	return execution.Registry{"slack.message": &messageRunner{credentials: credentials, client: client, allowed: isSlackWebhook}}
}

type messageRunner struct {
	credentials Credentials
	client      *http.Client
	allowed     func(*url.URL) bool
}

func (s *messageRunner) Run(ctx context.Context, in execution.Input) (execution.Result, error) {
	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(in.Data, &data); err != nil {
		return execution.Result{}, fmt.Errorf("slack message data: %w", err)
	}
	if data.Text == "" {
		return execution.Result{}, errors.New("slack message needs text")
	}

	secret, err := s.credentials.Get(ctx, in.WorkspaceID, credentialProvider, credentialName)
	if errors.Is(err, credential.ErrNotFound) {
		return execution.Result{}, ErrNoCredential
	}
	if err != nil {
		return execution.Result{}, fmt.Errorf("slack credential: %w", err)
	}
	endpoint, err := url.Parse(string(secret))
	if err != nil || !s.allowed(endpoint) {
		return execution.Result{}, errors.New("slack: credential slack/default is not an incoming webhook URL")
	}

	body, err := json.Marshal(map[string]string{"text": data.Text})
	if err != nil {
		return execution.Result{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return execution.Result{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return execution.Result{}, fmt.Errorf("slack message: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return execution.Result{}, fmt.Errorf("slack message: %s", response.Status)
	}
	return execution.Result{Output: json.RawMessage(`{"sent":true}`)}, nil
}

func isSlackWebhook(endpoint *url.URL) bool {
	return endpoint.Scheme == "https" && (endpoint.Hostname() == "hooks.slack.com" || endpoint.Hostname() == "hooks.slack-gov.com")
}
