package execution

import (
	"encoding/json"
	"time"
)

// Publisher is where the engine announces what it is doing.
//
// Declared here, where it is consumed, so this package does not import the
// websocket package and the websocket package does not import this one. Neither
// knows the other exists; main.go puts them together.
//
// It returns nothing and must not block. A run cannot be held up by whoever is
// watching it, and there is no useful way for the engine to handle a failure to
// notify — the row is the record, and the notification is a convenience.
type Publisher interface {
	Publish(topic string, event any)
}

// Topic is where an execution's events are published. The API layer subscribes
// with the same function, so the two cannot disagree about the string.
func Topic(executionID string) string { return "execution:" + executionID }

// Event types. Two, not one per status: a status is already a field, and a
// client switching on it needs to know whether the subject is the run or a node
// inside it.
const (
	EventExecution = "execution"
	EventNode      = "node"
)

// maxEventOutput caps the output carried on an event.
//
// A node's stored output is already bounded, but a megabyte fanned out to every
// watcher, buffered per client, is a different budget from a megabyte written
// once to a row. Past this the field is dropped and the client fetches the node
// over REST, which is where the whole output always lives.
const maxEventOutput = 64 << 10

// Event is one thing that happened. Every event carries the full state of its
// subject rather than a delta, so a client that missed one is corrected by the
// next rather than left drifting.
type Event struct {
	Type        string          `json:"type"`
	ExecutionID string          `json:"executionId"`
	Status      Status          `json:"status"`
	NodeID      string          `json:"nodeId,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
	Truncated   bool            `json:"outputTruncated,omitempty"`
	Error       string          `json:"error,omitempty"`
	At          time.Time       `json:"at"`
}

// publish sends one event for this run. The nil check lives here so the rest of
// the engine can call it unconditionally.
func (s *Service) publish(event Event) {
	if s.events == nil {
		return
	}
	if len(event.Output) > maxEventOutput {
		event.Output = nil
		event.Truncated = true
	}
	event.At = s.now()
	s.events.Publish(Topic(event.ExecutionID), event)
}
