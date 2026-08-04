package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Builtin returns the runners that need nothing but the standard library.
//
// Everything else belongs to the package that owns the integration — GitHub
// nodes in internal/github, Slack in internal/slack — because those carry an
// SDK, a token, and an API to keep up with. These two carry none of that, and a
// package holding one function is not a boundary, it is ceremony.
func Builtin(client *http.Client) Registry {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return Registry{
		"trigger.manual":   RunnerFunc(runTrigger),
		"trigger.webhook":  RunnerFunc(runTrigger),
		"trigger.schedule": RunnerFunc(runTrigger),
		"logic.condition":  RunnerFunc(runCondition),
		"http.request":     &httpRunner{client: client},
	}
}

// runTrigger starts the run. A trigger does no work; its output is the payload
// the run arrived with, so downstream nodes can reach it as .nodes.<id> as well
// as .trigger.
func runTrigger(_ context.Context, in Input) (Result, error) {
	return Result{Output: in.Trigger}, nil
}

// runCondition picks a branch.
//
// The condition is evaluated by the template layer before the runner sees it,
// so the node's data is {"value": "<already substituted>"} and this only has to
// decide what counts as true. That keeps the engine free of an expression
// language: {{ if eq .trigger.branch "main" }}true{{ else }}false{{ end }} is
// text/template doing the work, and it is the same syntax as everywhere else.
func runCondition(_ context.Context, in Input) (Result, error) {
	var data struct {
		Value any `json:"value"`
	}
	if len(in.Data) > 0 {
		if err := json.Unmarshal(in.Data, &data); err != nil {
			return Result{}, fmt.Errorf("condition data: %w", err)
		}
	}

	branch := "false"
	if truthy(data.Value) {
		branch = "true"
	}
	output, _ := json.Marshal(map[string]any{"branch": branch})
	return Result{Output: output, Branch: branch}, nil
}

// truthy is deliberately narrow. A template renders everything to text, so the
// realistic inputs are "true"/"false" and the odd number; anything unrecognised
// is false, because a condition that guesses is worse than one that does not
// fire.
func truthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "1":
			return true
		}
	}
	return false
}

// httpRunner calls an HTTP endpoint.
type httpRunner struct {
	client *http.Client
}

// maxResponseBytes caps what one node brings back. The body is stored, returned
// over HTTP, and streamed to a browser, so an endpoint answering with a
// gigabyte would take the execution viewer down with it.
const maxResponseBytes = 1 << 20

func (h *httpRunner) Run(ctx context.Context, in Input) (Result, error) {
	var data struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	}
	if err := json.Unmarshal(in.Data, &data); err != nil {
		return Result{}, fmt.Errorf("http node data: %w", err)
	}
	if data.URL == "" {
		return Result{}, fmt.Errorf("http node needs a url")
	}
	if data.Method == "" {
		data.Method = http.MethodGet
	}

	var body io.Reader
	if len(data.Body) > 0 {
		body = strings.NewReader(string(data.Body))
	}

	request, err := http.NewRequestWithContext(ctx, strings.ToUpper(data.Method), data.URL, body)
	if err != nil {
		return Result{}, err
	}
	if body != nil && data.Headers["Content-Type"] == "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range data.Headers {
		request.Header.Set(name, value)
	}

	response, err := h.client.Do(request)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return Result{}, err
	}

	// A JSON response is decoded so downstream templates can reach into it;
	// anything else stays a string. Guessing wrong here would mean
	// .nodes.x.field silently not resolving.
	var decoded any
	if json.Unmarshal(raw, &decoded) != nil {
		decoded = string(raw)
	}

	output, err := json.Marshal(map[string]any{
		"status": response.StatusCode,
		"body":   decoded,
	})
	if err != nil {
		return Result{}, err
	}

	// A 4xx or 5xx fails the node. The status is in the output either way, so a
	// workflow that wants to handle it can put a condition node downstream of a
	// node it expects to fail — but the default has to be that a failed call
	// stops the run rather than feeding an error page to the next step.
	if response.StatusCode >= 400 {
		return Result{Output: output}, fmt.Errorf("http %s %s: %s", request.Method, data.URL, response.Status)
	}
	return Result{Output: output}, nil
}
