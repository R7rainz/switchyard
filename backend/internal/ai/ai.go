package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/R7rainz/switchyard/backend/internal/credential"
)

// JSONSchema asks a provider to constrain the response to Schema. It stays on
// the request rather than the Provider interface so providers that cannot
// enforce schemas can still use the same completion contract.
type JSONSchema struct {
	Name   string
	Schema json.RawMessage
	Strict bool
}

// ErrNoCredential means the workspace has not stored a provider key. It is the
// one failure here that is the caller's to fix, so it gets its own sentinel.
var ErrNoCredential = errors.New("ai: no API key stored for this workspace")

// NoCredentialError keeps the selected provider in the actionable error while
// preserving errors.Is(err, ErrNoCredential) for callers.
type NoCredentialError struct{ Provider string }

func (e NoCredentialError) Error() string {
	provider := e.Provider
	if provider == "" {
		provider = ProviderOpenRouter
	}
	return ErrNoCredential.Error() + ": " + provider
}

func (e NoCredentialError) Unwrap() error { return ErrNoCredential }

// ErrProvider wraps anything the model provider returned or did. It never
// carries the API key, and no error below is built from one.
var ErrProvider = errors.New("ai: provider failed")

// Where a workspace's key is kept. OpenRouter is the default because one key
// reaches every model; any other provider is another implementation of Provider
// behind this same lookup, not a special case above it.
const (
	ProviderOpenRouter = "openrouter"
	ProviderOpenAI     = "openai"
	ProviderAnthropic  = "anthropic"
	ProviderGemini     = "gemini"

	// CredentialProvider is kept as the default credential key for existing
	// callers and stored workflows.
	CredentialProvider = ProviderOpenRouter
	CredentialName     = "default"
)

// DefaultModel is the OpenRouter default. Native providers have their own
// defaults in DefaultModelFor, while a workflow that pinned a model keeps it.
const DefaultModel = "anthropic/claude-sonnet-4.5"

// Request is one completion. Two roles rather than a message list: nothing here
// holds a conversation yet, and a slice of messages with only ever two entries
// is a shape that invites callers to invent turn-taking this package does not
// support.
type Request struct {
	// Provider selects which workspace credential and adapter to use. Empty
	// keeps the OpenRouter default for existing workflows.
	Provider string
	Model    string
	System   string
	Prompt   string

	// MaxTokens bounds the reply. Zero leaves it to the provider.
	MaxTokens int

	// JSON asks the provider for a JSON object. It is a hint, not a guarantee —
	// every caller still has to parse defensively.
	JSON bool

	// JSONSchema is stronger than JSON when the provider supports structured
	// outputs. It takes precedence over JSON.
	JSONSchema *JSONSchema
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
// package talks to a provider, so selecting a provider never leaks into callers.
type Service struct {
	providers map[string]Provider
	creds     Credentials
}

func NewService(provider Provider, creds Credentials) *Service {
	return NewServiceWithProviders(map[string]Provider{ProviderOpenRouter: provider}, creds)
}

func NewServiceWithProviders(providers map[string]Provider, creds Credentials) *Service {
	return &Service{providers: providers, creds: creds}
}

// Complete runs one completion using workspaceID's own key.
//
// The key is fetched per call rather than cached: a workspace that rotates or
// revokes its key expects the next run to use the new one, and a cache here
// would hold plaintext for the life of the process.
func (s *Service) Complete(ctx context.Context, workspaceID string, req Request) (Response, error) {
	providerName := req.Provider
	if providerName == "" {
		providerName = ProviderOpenRouter
	}
	provider, ok := s.providers[providerName]
	if !ok || provider == nil {
		return Response{}, fmt.Errorf("%w: unsupported provider %q", ErrProvider, providerName)
	}

	secret, err := s.creds.Get(ctx, workspaceID, providerName, CredentialName)
	if err != nil {
		if errors.Is(err, credential.ErrNotFound) {
			return Response{}, NoCredentialError{Provider: providerName}
		}
		return Response{}, err
	}

	if req.Model == "" {
		req.Model = DefaultModelFor(providerName)
	}
	return provider.Complete(ctx, string(secret), req)
}

func DefaultModelFor(provider string) string {
	switch provider {
	case ProviderOpenAI:
		return "gpt-4o-mini"
	case ProviderAnthropic:
		return "claude-sonnet-5"
	case ProviderGemini:
		return "gemini-3.6-flash"
	default:
		return DefaultModel
	}
}
