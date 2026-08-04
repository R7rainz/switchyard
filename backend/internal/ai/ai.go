package ai

import (
	"context"
	"errors"

	"github.com/R7rainz/switchyard/backend/internal/credential"
)

// ErrNoCredential means the workspace has not stored a provider key. It is the
// one failure here that is the caller's to fix, so it gets its own sentinel.
var ErrNoCredential = errors.New("ai: no API key stored for this workspace")

// ErrProvider wraps anything the model provider returned or did. It never
// carries the API key, and no error below is built from one.
var ErrProvider = errors.New("ai: provider failed")

// Where a workspace's key is kept. OpenRouter is the default because one key
// reaches every model; any other provider is another implementation of Provider
// behind this same lookup, not a special case above it.
const (
	CredentialProvider = "openrouter"
	CredentialName     = "default"
)

// DefaultModel is used when a node or request does not name one. It is a single
// constant on purpose — changing the platform default should be one edit, and
// a workflow that pinned a model keeps it.
const DefaultModel = "anthropic/claude-sonnet-4.5"

// Request is one completion. Two roles rather than a message list: nothing here
// holds a conversation yet, and a slice of messages with only ever two entries
// is a shape that invites callers to invent turn-taking this package does not
// support.
type Request struct {
	Model  string
	System string
	Prompt string

	// MaxTokens bounds the reply. Zero leaves it to the provider.
	MaxTokens int

	// JSON asks the provider for a JSON object. It is a hint, not a guarantee —
	// every caller still has to parse defensively.
	JSON bool
}

// Response is what came back. Model is the model that actually answered, which
// can differ from the one asked for when a provider routes around an outage.
type Response struct {
	Text  string
	Model string
}

// Provider is one model vendor. The key is a parameter rather than a field
// because it belongs to a workspace, not to the process: one Provider serves
// every workspace, and no long-lived struct ends up holding a secret.
type Provider interface {
	Complete(ctx context.Context, key string, req Request) (Response, error)
}

// Credentials is the part of the credential service this package needs,
// declared here where it is consumed.
type Credentials interface {
	Get(ctx context.Context, workspaceID, provider, name string) (credential.Secret, error)
}

// Service is the AI surface the rest of Switchyard calls. Nothing outside this
// package talks to a provider, so swapping OpenRouter for anything else is one
// constructor argument.
type Service struct {
	provider Provider
	creds    Credentials
}

func NewService(provider Provider, creds Credentials) *Service {
	return &Service{provider: provider, creds: creds}
}

// Complete runs one completion using workspaceID's own key.
//
// The key is fetched per call rather than cached: a workspace that rotates or
// revokes its key expects the next run to use the new one, and a cache here
// would hold plaintext for the life of the process.
func (s *Service) Complete(ctx context.Context, workspaceID string, req Request) (Response, error) {
	secret, err := s.creds.Get(ctx, workspaceID, CredentialProvider, CredentialName)
	if err != nil {
		if errors.Is(err, credential.ErrNotFound) {
			return Response{}, ErrNoCredential
		}
		return Response{}, err
	}

	if req.Model == "" {
		req.Model = DefaultModel
	}
	return s.provider.Complete(ctx, string(secret), req)
}
