package execution

import (
	"context"
	"encoding/json"
	"errors"
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
// SDK, a token, and an API to keep up with. These carry none of that, and a
// package holding one function is not a boundary, it is ceremony.
func Builtin(client *http.Client) Registry {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	client = newSSRFProtectedClient(client, lookupHost)
	return Registry{
		"trigger.manual":              RunnerFunc(runTrigger),
		"trigger.webhook":             RunnerFunc(runTrigger),
		"trigger.schedule":            RunnerFunc(runTrigger),
		"trigger.github.pull_request": RunnerFunc(runTrigger),
		"logic.condition":             RunnerFunc(runCondition),
		"logic.switch":                RunnerFunc(runSwitch),
		"logic.delay":                 RunnerFunc(runDelay),
		"variable.set":                RunnerFunc(runSetVariable),
		"http.request":                &httpRunner{client: client, lookupIP: lookupHost},
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

// runSwitch compares one rendered value with the configured case labels. The
// matching label becomes the source handle; values without a match use the
// explicit default handle so a workflow can keep its fallback path visible.
func runSwitch(_ context.Context, in Input) (Result, error) {
	var data struct {
		Value any             `json:"value"`
		Cases json.RawMessage `json:"cases"`
	}
	if len(in.Data) > 0 {
		if err := json.Unmarshal(in.Data, &data); err != nil {
			return Result{}, fmt.Errorf("switch data: %w", err)
		}
	}
	cases, err := stringArray(data.Cases)
	if err != nil {
		return Result{}, fmt.Errorf("switch node cases: %w", err)
	}
	if len(cases) == 0 {
		return Result{}, fmt.Errorf("switch node needs at least one case")
	}
	seen := make(map[string]struct{}, len(cases))
	for _, candidate := range cases {
		if strings.TrimSpace(candidate) == "" {
			return Result{}, fmt.Errorf("switch node cases cannot be empty")
		}
		if _, duplicate := seen[candidate]; duplicate {
			return Result{}, fmt.Errorf("switch node case %q is duplicated", candidate)
		}
		if candidate == "default" {
			return Result{}, fmt.Errorf("switch node case %q is reserved", candidate)
		}
		seen[candidate] = struct{}{}
	}

	branch := "default"
	value := switchValue(data.Value)
	for _, candidate := range cases {
		if candidate == value {
			branch = candidate
			break
		}
	}
	output, err := json.Marshal(map[string]any{"value": data.Value, "branch": branch})
	if err != nil {
		return Result{}, err
	}
	return Result{Output: output, Branch: branch}, nil
}

func switchValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func stringArray(raw json.RawMessage) ([]string, error) {
	raw = unwrapJSONText(raw)
	var values []string
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("must be a JSON array of strings: %w", err)
	}
	return values, nil
}

// unwrapJSONText accepts the frontend's editable JSON fields while a user is
// typing. Completed fields are objects/arrays; half-edited fields remain JSON
// strings until the next save, so a run can report the actual field error.
func unwrapJSONText(raw json.RawMessage) json.RawMessage {
	var text string
	if len(raw) > 0 && json.Unmarshal(raw, &text) == nil {
		return json.RawMessage(text)
	}
	return raw
}

// runDelay pauses without blocking the engine's goroutine or ignoring cancel.
// The engine's per-node timeout remains the upper bound for a long duration.
func runDelay(ctx context.Context, in Input) (Result, error) {
	var data struct {
		Duration string `json:"duration"`
	}
	if len(in.Data) > 0 {
		if err := json.Unmarshal(in.Data, &data); err != nil {
			return Result{}, fmt.Errorf("delay data: %w", err)
		}
	}
	duration, err := time.ParseDuration(strings.TrimSpace(data.Duration))
	if err != nil || duration <= 0 {
		if err == nil {
			err = errors.New("duration must be greater than zero")
		}
		return Result{}, fmt.Errorf("delay node needs a valid duration: %w", err)
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		output, _ := json.Marshal(map[string]string{"duration": duration.String()})
		return Result{Output: output}, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
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

// runSetVariable names values for the nodes downstream of it.
//
// It computes nothing: the template layer has already substituted the node's
// data, so this only decides what the node's output is. The point is a name — a
// later node reads {{ .nodes.pr.number }} instead of repeating the expression
// that produced it, and the value is visible on the canvas and on the run,
// which is the difference between a workflow you can read and one you cannot.
func runSetVariable(_ context.Context, in Input) (Result, error) {
	var data struct {
		Values json.RawMessage `json:"values"`
	}
	if len(in.Data) > 0 {
		if err := json.Unmarshal(in.Data, &data); err != nil {
			return Result{}, fmt.Errorf("variable node data: %w", err)
		}
	}
	values := unwrapJSONText(data.Values)
	if len(values) == 0 {
		return Result{}, fmt.Errorf("variable node needs a values object")
	}

	// It has to be an object, because the whole purpose is to be reached into
	// as .nodes.<id>.<name>. A list or a bare string would fail later, in a
	// template, with a message about the node that referred to it rather than
	// the node that is wrong.
	var named map[string]any
	if err := json.Unmarshal(values, &named); err != nil {
		return Result{}, fmt.Errorf("variable node values must be an object of name/value pairs: %w", err)
	}

	// The output is the values themselves rather than a wrapper, so a reference
	// is .nodes.<id>.<name> — the same shape as every other node's output.
	return Result{Output: values}, nil
}

// httpRunner calls an HTTP endpoint.
type httpRunner struct {
	client   *http.Client
	lookupIP lookupIPFunc
}

// maxResponseBytes caps what one node brings back. The body is stored, returned
// over HTTP, and streamed to a browser, so an endpoint answering with a
// gigabyte would take the execution viewer down with it.
const maxResponseBytes = 1 << 20

func (h *httpRunner) Run(ctx context.Context, in Input) (Result, error) {
	var data struct {
		Method  string          `json:"method"`
		URL     string          `json:"url"`
		Headers json.RawMessage `json:"headers"`
		Body    json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(in.Data, &data); err != nil {
		return Result{}, fmt.Errorf("http node data: %w", err)
	}
	if data.URL == "" {
		return Result{}, fmt.Errorf("http node needs a url")
	}
	if err := validateHTTPURL(ctx, data.URL, h.lookupIP); err != nil {
		return Result{}, err
	}
	if data.Method == "" {
		data.Method = http.MethodGet
	}

	headers := map[string]string{}
	if raw := unwrapJSONText(data.Headers); len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &headers); err != nil {
			return Result{}, fmt.Errorf("http node headers must be a JSON object: %w", err)
		}
	}

	var body io.Reader
	bodyData := unwrapJSONText(data.Body)
	if len(bodyData) > 0 {
		body = strings.NewReader(string(bodyData))
	}

	request, err := http.NewRequestWithContext(ctx, strings.ToUpper(data.Method), data.URL, body)
	if err != nil {
		return Result{}, err
	}
	if body != nil && headers["Content-Type"] == "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	client := h.client
	if client == nil {
		client = newSSRFProtectedClient(nil, h.lookupIP)
	} else {
		// Do not mutate a shared client: runners may execute concurrently, and
		// a redirect policy belongs to this request rather than global state.
		copy := *client
		copy.CheckRedirect = rejectRedirect
		client = &copy
	}
	response, err := client.Do(request)
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
