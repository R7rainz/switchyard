package execution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// interpolate substitutes earlier nodes' output into a node's data.
//
//	{"url": "https://api.example.com/repos/{{ .trigger.repo }}/issues"}
//	{"body": "{{ .nodes.summarise.text }}"}
//
// text/template rather than an expression language of our own: it is standard
// library, the syntax is one every Go developer already reads, and the
// alternative is a parser to write, document, and get wrong. A node that wants
// something cleverer can read Input.Outputs directly.
//
// Missing keys are an error rather than an empty string. A workflow that
// silently posts "" where a pull request number should be is worse than one
// that fails and says which reference was wrong.
func interpolate(data json.RawMessage, outputs map[string]json.RawMessage, trigger json.RawMessage) (json.RawMessage, error) {
	if len(data) == 0 || !bytes.Contains(data, []byte("{{")) {
		// Nothing to substitute. Worth checking, because the common node has no
		// templates at all and this skips a parse and a re-encode.
		return data, nil
	}

	context, err := templateContext(outputs, trigger)
	if err != nil {
		return nil, err
	}

	// The template is applied to the JSON text, so a substituted value lands
	// inside the string that quoted it. That means a value containing a quote
	// would break the document, which is what escapeJSON below prevents.
	parsed, err := template.New("data").
		Option("missingkey=error").
		Funcs(template.FuncMap{"json": escapeJSON}).
		Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("node data template: %w", err)
	}

	var rendered bytes.Buffer
	if err := parsed.Execute(&rendered, context); err != nil {
		return nil, fmt.Errorf("node data: %w", err)
	}

	if !json.Valid(rendered.Bytes()) {
		// Almost always a substituted value carrying a quote or a newline. The
		// message points at the fix rather than at the JSON parser.
		return nil, fmt.Errorf("node data: substitution produced invalid JSON; wrap the reference in {{ json . }} if the value can contain quotes")
	}
	return rendered.Bytes(), nil
}

// templateContext is what a template sees:
//
//	.trigger   the payload the run started with
//	.nodes     every finished node's output, keyed by node id
func templateContext(outputs map[string]json.RawMessage, trigger json.RawMessage) (map[string]any, error) {
	nodes := make(map[string]any, len(outputs))
	for id, output := range outputs {
		value, err := decode(output)
		if err != nil {
			return nil, fmt.Errorf("output of node %q: %w", id, err)
		}
		nodes[id] = value
	}

	payload, err := decode(trigger)
	if err != nil {
		return nil, fmt.Errorf("trigger payload: %w", err)
	}

	return map[string]any{"nodes": nodes, "trigger": payload}, nil
}

// decode turns stored JSON into something a template can walk. A nil or empty
// value becomes an empty map rather than nil, so referring to a field of a node
// that returned nothing is a missing-key error naming the field instead of a
// nil dereference.
func decode(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]any{}, nil
	}
	return value, nil
}

// escapeJSON renders a value as a JSON string body, so it can be dropped
// between the quotes already in the template without ending the string early.
// Exposed to templates as {{ json .nodes.x.y }}.
func escapeJSON(value any) (string, error) {
	encoded, err := json.Marshal(fmt.Sprint(value))
	if err != nil {
		return "", err
	}
	// Marshal gives back a quoted string; the template already has the quotes.
	return strings.TrimSuffix(strings.TrimPrefix(string(encoded), `"`), `"`), nil
}
