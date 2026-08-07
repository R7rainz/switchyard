package discord

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

const credentialProvider = "discord"
const credentialName = "default"

var ErrNoCredential = errors.New("discord: missing credential discord/default")

type Credentials interface {
	Get(context.Context, string, string, string) (credential.Secret, error)
}

type messageRunner struct {
	credentials Credentials
	client      *http.Client
	allowed     func(*url.URL) bool
}

func Runners(credentials Credentials, client *http.Client) execution.Registry {
	if client == nil {
		client = http.DefaultClient
	}
	return execution.Registry{"discord.message": &messageRunner{
		credentials: credentials, client: client, allowed: isDiscordWebhook,
	}}
}

func (d *messageRunner) Run(ctx context.Context, in execution.Input) (execution.Result, error) {
	var data map[string]string
	if err := json.Unmarshal(in.Data, &data); err != nil {
		return execution.Result{}, fmt.Errorf("discord message data: %w", err)
	}
	text := strings.TrimSpace(data["text"])
	if text == "" {
		return execution.Result{}, errors.New("discord message needs text")
	}
	secret, err := d.credentials.Get(ctx, in.WorkspaceID, credentialProvider, credentialName)
	if errors.Is(err, credential.ErrNotFound) {
		return execution.Result{}, ErrNoCredential
	}
	if err != nil {
		return execution.Result{}, fmt.Errorf("discord credential: %w", err)
	}
	endpoint, err := url.Parse(string(secret))
	if err != nil || !d.allowed(endpoint) {
		return execution.Result{}, errors.New("discord: credential discord/default is not a webhook URL")
	}
	body, _ := json.Marshal(map[string]string{"content": text})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return execution.Result{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := d.client.Do(request)
	if err != nil {
		return execution.Result{}, fmt.Errorf("discord message: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return execution.Result{}, fmt.Errorf("discord message: %s", response.Status)
	}
	return execution.Result{Output: json.RawMessage("{\"sent\":true}")}, nil
}

func isDiscordWebhook(endpoint *url.URL) bool {
	if endpoint == nil || endpoint.Scheme != "https" {
		return false
	}
	host := endpoint.Hostname()
	return (host == "discord.com" || host == "discordapp.com") &&
		strings.HasPrefix(endpoint.Path, "/api/webhooks/")
}
