package ai

import (
	"context"
	"errors"

	"github.com/R7rainz/switchyard/backend/internal/credential"
)

// stubProvider answers with whatever the test queued, and records what it was
// asked, which is how the retry is observed.
type stubProvider struct {
	replies []string
	err     error
	prompts []string
	keys    []string
}

func (s *stubProvider) Complete(_ context.Context, key string, req Request) (Response, error) {
	s.prompts = append(s.prompts, req.Prompt)
	s.keys = append(s.keys, key)
	if s.err != nil {
		return Response{}, s.err
	}
	if len(s.replies) == 0 {
		return Response{}, errors.New("stub: no reply queued")
	}
	reply := s.replies[0]
	s.replies = s.replies[1:]
	return Response{Text: reply, Model: req.Model}, nil
}

// stubCreds stands in for the credential service. An empty key means the
// workspace has stored nothing.
type stubCreds struct{ key string }

func (s stubCreds) Get(context.Context, string, string, string) (credential.Secret, error) {
	if s.key == "" {
		return nil, credential.ErrNotFound
	}
	return credential.Secret(s.key), nil
}
