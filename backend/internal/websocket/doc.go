// Package websocket streams live execution logs to connected clients.
//
// It is a transport, not a source of truth: logs are persisted by the
// execution package and merely relayed here.
package websocket
