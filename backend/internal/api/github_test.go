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
