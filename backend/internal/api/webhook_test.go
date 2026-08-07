package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/credential"
)

func TestGenericWebhookSignatureValidation(t *testing.T) {
	secret, body := credential.Secret("webhook-secret"), []byte(`{"event":"push"}`)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	valid := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !webhookSignatureValid(secret, valid, body) {
		t.Fatal("valid generic webhook signature was rejected")
	}
	if webhookSignatureValid(secret, valid, []byte("different")) {
		t.Fatal("signature was accepted for a different body")
	}
}
