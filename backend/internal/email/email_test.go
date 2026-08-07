package email

import (
	"context"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/execution"
)

type testCredentials map[string]credential.Secret

func (c testCredentials) Get(_ context.Context, _ string, provider, name string) (credential.Secret, error) {
	if secret, ok := c[provider+"/"+name]; ok {
		return secret, nil
	}
	return nil, credential.ErrNotFound
}

func TestMessageRunnerValidatesSMTPCredential(t *testing.T) {
	runner := Runners(testCredentials{"email/default": credential.Secret("{\"host\":\"smtp.example.com\",\"from\":\"bot@example.com\"}")})["email.message"]
	_, err := runner.Run(t.Context(), execution.Input{WorkspaceID: "ws", Data: []byte("{\"to\":\"dev@example.com\",\"subject\":\"hello\",\"text\":\"body\"}")})
	if err == nil {
		t.Fatal("expected SMTP delivery to fail without a reachable server")
	}
}

func TestMessageRunnerRejectsHeaderInjection(t *testing.T) {
	runner := Runners(testCredentials{"email/default": credential.Secret(`{"host":"smtp.example.com","from":"bot@example.com"}`)})["email.message"]
	if _, err := runner.Run(t.Context(), execution.Input{WorkspaceID: "ws", Data: []byte(`{"to":"dev@example.com","subject":"hello\r\nBcc:bad@example.com","text":"body"}`)}); err == nil {
		t.Fatal("accepted a subject containing a newline")
	}
}
