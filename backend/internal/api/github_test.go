package api

import (
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/credential"
)

func TestGitHubSignatureValidation(t *testing.T) {
	body := []byte("Hello, World!")
	signature := "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"
	if !githubSignatureValid(credential.Secret("It's a Secret to Everybody"), signature, body) {
		t.Fatal("valid GitHub signature was rejected")
	}
	if githubSignatureValid(credential.Secret("wrong"), signature, body) {
		t.Fatal("invalid GitHub signature was accepted")
	}
}

func TestGitHubDeliveryKeyIsWorkflowScoped(t *testing.T) {
	first := githubDeliveryKey("workflow-a", "delivery-1")
	second := githubDeliveryKey("workflow-b", "delivery-1")
	if first == second {
		t.Fatalf("delivery keys collided across workflows: %q", first)
	}
	if got := githubDeliveryKey("workflow-a", "  delivery-1 "); got != first {
		t.Fatalf("delivery key did not normalize whitespace: %q", got)
	}
	if got := githubDeliveryKey("workflow-a", ""); got != "" {
		t.Fatalf("missing delivery id = %q, want no idempotency key", got)
	}
}
