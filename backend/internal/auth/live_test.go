package auth

import (
	"context"
	"os"
	"testing"
)

// TestVerifyAgainstLiveIssuer checks this verifier against a running Better
// Auth server rather than captured bytes. It is the only test that proves the
// two languages agree end to end, so it is worth running by hand after any
// change to the auth config on either side.
//
// Skipped unless both are set:
//
//	SWITCHYARD_LIVE_ISSUER  base URL of the running frontend
//	SWITCHYARD_LIVE_TOKEN   a token minted by GET /api/auth/token
func TestVerifyAgainstLiveIssuer(t *testing.T) {
	issuer := os.Getenv("SWITCHYARD_LIVE_ISSUER")
	token := os.Getenv("SWITCHYARD_LIVE_TOKEN")
	if issuer == "" || token == "" {
		t.Skip("set SWITCHYARD_LIVE_ISSUER and SWITCHYARD_LIVE_TOKEN to run")
	}

	v := NewVerifier(issuer+"/api/auth/jwks", issuer, testAudience)

	claims, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify against %s: %v", issuer, err)
	}
	if claims.Subject == "" {
		t.Fatal("verified token carried no subject")
	}
	t.Logf("verified live token for subject %s (%s), expires %s",
		claims.Subject, claims.Email, claims.ExpiresAt)
}
