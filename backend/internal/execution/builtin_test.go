package execution

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/workflow"
)

func TestInterpolate(t *testing.T) {
	outputs := map[string]json.RawMessage{
		"fetch": json.RawMessage(`{"body":{"name":"switchyard","count":3},"status":200}`),
	}
	trigger := json.RawMessage(`{"repo":"main","pr":42}`)

	cases := map[string]struct{ data, want string }{
		"no template is passed through untouched": {
			`{"url":"https://example.com"}`,
			`{"url":"https://example.com"}`,
		},
		"a node's output": {
			`{"text":"{{ .nodes.fetch.body.name }}"}`,
			`{"text":"switchyard"}`,
		},
		"the trigger payload": {
			`{"text":"pr {{ .trigger.pr }}"}`,
			`{"text":"pr 42"}`,
		},
		"several in one string": {
			`{"text":"{{ .nodes.fetch.body.name }} #{{ .trigger.pr }}"}`,
			`{"text":"switchyard #42"}`,
		},
		"a number keeps its type when unquoted": {
			`{"count":{{ .nodes.fetch.body.count }}}`,
			`{"count":3}`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := interpolate(json.RawMessage(tc.data), outputs, trigger)
			if err != nil {
				t.Fatalf("interpolate: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// A reference that resolves to nothing is an error, not an empty string. A
// workflow that posts "" where a PR number belongs is worse than one that stops.
func TestInterpolateRejectsMissingReferences(t *testing.T) {
	cases := map[string]string{
		"unknown node":  `{"text":"{{ .nodes.ghost.field }}"}`,
		"unknown field": `{"text":"{{ .trigger.nothing }}"}`,
		"bad syntax":    `{"text":"{{ .nodes. }}"}`,
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := interpolate(json.RawMessage(data), nil, json.RawMessage(`{"a":1}`)); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

// A value carrying a quote would otherwise end the JSON string early and turn
// one field into several. This is the case the json helper exists for.
func TestInterpolateCatchesBrokenSubstitution(t *testing.T) {
	outputs := map[string]json.RawMessage{
		"a": json.RawMessage(`{"text":"he said \"hello\""}`),
	}

	_, err := interpolate(json.RawMessage(`{"say":"{{ .nodes.a.text }}"}`), outputs, nil)
	if err == nil {
		t.Fatal("a quote in a substituted value should not pass silently")
	}
	if !strings.Contains(err.Error(), "json") {
		t.Fatalf("error %q should point at the fix", err)
	}

	// And the escaped form works.
	got, err := interpolate(json.RawMessage(`{"say":"{{ json .nodes.a.text }}"}`), outputs, nil)
	if err != nil {
		t.Fatalf("escaped substitution: %v", err)
	}
	var decoded struct{ Say string }
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("result is not valid JSON: %s", got)
	}
	if decoded.Say != `he said "hello"` {
		t.Fatalf("say = %q", decoded.Say)
	}
}

func TestHTTPRunner(t *testing.T) {
	var gotMethod, gotBody, gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotHeader = r.Header.Get("X-Token")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)

		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"switchyard"}`))
	}))
	defer server.Close()

	// The test server is intentionally loopback. Give the runner a fake public
	// resolution so this test exercises HTTP handling without weakening the
	// production SSRF policy.
	runner := &httpRunner{
		client: server.Client(),
		lookupIP: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		},
	}

	t.Run("decodes a JSON response for downstream nodes", func(t *testing.T) {
		data := `{"method":"post","url":"` + server.URL + `","headers":{"X-Token":"abc"},"body":{"hello":"world"}}`
		result, err := runner.Run(context.Background(), Input{Data: json.RawMessage(data)})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		var out struct {
			Status int `json:"status"`
			Body   struct {
				Name string `json:"name"`
			} `json:"body"`
		}
		if err := json.Unmarshal(result.Output, &out); err != nil {
			t.Fatalf("output: %v", err)
		}
		if out.Status != 200 || out.Body.Name != "switchyard" {
			t.Fatalf("output = %s", result.Output)
		}
		if gotMethod != http.MethodPost || gotHeader != "abc" || gotBody != `{"hello":"world"}` {
			t.Fatalf("request was %s, header %q, body %q", gotMethod, gotHeader, gotBody)
		}
	})

	// A 500 fails the node, and the body is kept: it is usually the whole
	// explanation of what went wrong.
	t.Run("a 5xx fails but keeps the body", func(t *testing.T) {
		data := `{"url":"` + server.URL + `/fail"}`
		result, err := runner.Run(context.Background(), Input{Data: json.RawMessage(data)})
		if err == nil {
			t.Fatal("a 500 should fail the node")
		}
		if !strings.Contains(string(result.Output), "boom") {
			t.Fatalf("output = %s, want the response body kept", result.Output)
		}
	})

	t.Run("a missing url is rejected", func(t *testing.T) {
		if _, err := runner.Run(context.Background(), Input{Data: json.RawMessage(`{}`)}); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestCondition(t *testing.T) {
	cases := map[string]string{
		`{"value":"true"}`:  "true",
		`{"value":true}`:    "true",
		`{"value":"yes"}`:   "true",
		`{"value":"1"}`:     "true",
		`{"value":"false"}`: "false",
		`{"value":""}`:      "false",
		`{"value":"maybe"}`: "false", // unrecognised is false: a condition must not guess
		`{}`:                "false",
	}

	for data, want := range cases {
		result, err := runCondition(context.Background(), Input{Data: json.RawMessage(data)})
		if err != nil {
			t.Fatalf("%s: %v", data, err)
		}
		if result.Branch != want {
			t.Errorf("%s: branch = %q, want %q", data, result.Branch, want)
		}
	}
}

// A variable node names values for the nodes after it, so a later reference is
// .nodes.<id>.<name> — the same shape as every other node's output, rather than
// a wrapper only this node type has.
func TestSetVariable(t *testing.T) {
	result, err := runSetVariable(t.Context(), Input{
		Data: json.RawMessage(`{"label":"Facts","values":{"repo":"switchyard","number":42}}`),
	})
	if err != nil {
		t.Fatalf("runSetVariable: %v", err)
	}

	var named map[string]any
	if err := json.Unmarshal(result.Output, &named); err != nil {
		t.Fatalf("output is not an object: %s", result.Output)
	}
	if named["repo"] != "switchyard" || named["number"] != float64(42) {
		t.Fatalf("output = %s", result.Output)
	}
	// The canvas label is not a variable.
	if _, leaked := named["label"]; leaked {
		t.Fatalf("the node's label leaked into its output: %s", result.Output)
	}
	if result.Branch != "" {
		t.Fatalf("a variable node picked a branch: %q", result.Branch)
	}
}

// Values that are not an object would fail later, inside a template, naming the
// node that referred to them rather than the node that is wrong.
func TestSetVariableRejectsWhatCannotBeNamed(t *testing.T) {
	cases := map[string]string{
		"no values at all": `{"label":"Facts"}`,
		"empty data":       ``,
		"a list":           `{"values":["a","b"]}`,
		"a bare string":    `{"values":"switchyard"}`,
		"a number":         `{"values":42}`,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := runSetVariable(t.Context(), Input{Data: json.RawMessage(data)}); err == nil {
				t.Fatal("accepted values that cannot be reached into")
			}
		})
	}
}

// The whole point: a later node reads a value by the name this one gave it.
func TestVariableFlowsIntoTheNextNode(t *testing.T) {
	graph := workflow.Graph{
		Nodes: []workflow.Node{
			node("t", "trigger.manual", ""),
			node("v", "variable.set", `{"values":{"repo":"switchyard"}}`),
			node("a", "http.request", `{"url":"https://example.com/{{ .nodes.v.repo }}"}`),
		},
		Edges: []workflow.Edge{
			{ID: "e1", Source: "t", Target: "v"},
			{ID: "e2", Source: "v", Target: "a"},
		},
	}

	// A local runner rather than the shared recorder: what matters here is the
	// data the node was handed after interpolation, which is the one thing the
	// recorder does not keep.
	var seen string
	runners := Registry{
		"trigger.manual": RunnerFunc(runTrigger),
		"variable.set":   RunnerFunc(runSetVariable),
		"http.request": RunnerFunc(func(_ context.Context, in Input) (Result, error) {
			seen = string(in.Data)
			return Result{Output: json.RawMessage(`{}`)}, nil
		}),
	}
	store := NewMemoryStore()
	svc := NewService(store, stubWorkflows{graph: graph}, runners, Options{})

	run, err := svc.Start(t.Context(), "ws", "wf", "alice", TriggerManual, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := await(t, store, "ws", run.ID); got.Status != StatusSucceeded {
		t.Fatalf("status = %s: %s", got.Status, got.Error)
	}

	if want := "https://example.com/switchyard"; !strings.Contains(seen, want) {
		t.Fatalf("the variable did not reach the next node: %q", seen)
	}
}
