// Package credential stores third-party secrets — provider API keys and OAuth
// tokens — encrypted with AES-256-GCM under a versioned master key. Plaintext
// exists only in memory, only for the workspace that asked for it, and never in a
// log line, an error, or a stored row.
package credential
