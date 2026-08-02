package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testIssuer   = "http://localhost:3000"
	testAudience = "switchyard-backend"
	testKid      = "test-key-1"
)

// jwksServer publishes pub under kid and counts how often it is asked.
func jwksServer(t *testing.T, keys map[string]ed25519.PublicKey, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		document := struct {
			Keys []map[string]string `json:"keys"`
		}{}
		for kid, pub := range keys {
			document.Keys = append(document.Keys, map[string]string{
				"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "kid": kid,
				"x": base64.RawURLEncoding.EncodeToString(pub),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(document)
	}))
	t.Cleanup(server.Close)
	return server
}

// mint builds a token, signing with priv unless the caller overrides the parts.
func mint(t *testing.T, priv ed25519.PrivateKey, header, payload map[string]any) string {
	t.Helper()
	encode := func(value map[string]any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshalling %v: %v", value, err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	signingInput := encode(header) + "." + encode(payload)
	signature := ed25519.Sign(priv, []byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func validPayload(now time.Time) map[string]any {
	return map[string]any{
		"sub":   "user-123",
		"iss":   testIssuer,
		"aud":   testAudience,
		"iat":   now.Unix(),
		"exp":   now.Add(15 * time.Minute).Unix(),
		"email": "dev@switchyard.test",
		"name":  "Dev",
	}
}

func newTestVerifier(t *testing.T, pub ed25519.PublicKey, now time.Time) *Verifier {
	t.Helper()
	server := jwksServer(t, map[string]ed25519.PublicKey{testKid: pub}, nil)
	v := NewVerifier(server.URL, testIssuer, testAudience)
	v.now = func() time.Time { return now }
	return v
}

func TestVerifyAcceptsValidToken(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	v := newTestVerifier(t, pub, now)

	token := mint(t, priv, map[string]any{"alg": "EdDSA", "kid": testKid}, validPayload(now))

	claims, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("Subject = %q, want user-123", claims.Subject)
	}
	if claims.Email != "dev@switchyard.test" {
		t.Errorf("Email = %q, want dev@switchyard.test", claims.Email)
	}
	if !claims.ExpiresAt.After(now) {
		t.Errorf("ExpiresAt = %v, want after %v", claims.ExpiresAt, now)
	}
}

func TestVerifyRejects(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, otherPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()

	validHeader := map[string]any{"alg": "EdDSA", "kid": testKid}

	tests := []struct {
		name  string
		token func() string
		want  string
	}{
		{
			name: "expired",
			token: func() string {
				p := validPayload(now)
				p["exp"] = now.Add(-2 * time.Hour).Unix()
				return mint(t, priv, validHeader, p)
			},
			want: "expired",
		},
		{
			name: "not yet valid",
			token: func() string {
				p := validPayload(now)
				p["nbf"] = now.Add(2 * time.Hour).Unix()
				return mint(t, priv, validHeader, p)
			},
			want: "not valid until",
		},
		{
			name: "no expiry",
			token: func() string {
				p := validPayload(now)
				delete(p, "exp")
				return mint(t, priv, validHeader, p)
			},
			want: "no expiry",
		},
		{
			name: "wrong issuer",
			token: func() string {
				p := validPayload(now)
				p["iss"] = "https://evil.example"
				return mint(t, priv, validHeader, p)
			},
			want: "issuer",
		},
		{
			name: "wrong audience",
			token: func() string {
				p := validPayload(now)
				p["aud"] = "some-other-service"
				return mint(t, priv, validHeader, p)
			},
			want: "audience",
		},
		{
			name: "missing subject",
			token: func() string {
				p := validPayload(now)
				delete(p, "sub")
				return mint(t, priv, validHeader, p)
			},
			want: "no subject",
		},
		{
			name: "signed by another key",
			token: func() string {
				return mint(t, otherPriv, validHeader, validPayload(now))
			},
			want: "signature does not verify",
		},
		{
			name: "alg none",
			token: func() string {
				p := validPayload(now)
				encode := func(v map[string]any) string {
					raw, _ := json.Marshal(v)
					return base64.RawURLEncoding.EncodeToString(raw)
				}
				return encode(map[string]any{"alg": "none", "kid": testKid}) + "." + encode(p) + "."
			},
			want: "rejected",
		},
		{
			name: "alg HS256 with the public key as the HMAC secret",
			token: func() string {
				// The classic confusion attack: the verifier must refuse on alg
				// alone, before any key material is chosen.
				token := mint(t, priv, map[string]any{"alg": "HS256", "kid": testKid}, validPayload(now))
				return token
			},
			want: "rejected",
		},
		{
			name: "tampered payload",
			token: func() string {
				token := mint(t, priv, validHeader, validPayload(now))
				parts := strings.Split(token, ".")
				forged := validPayload(now)
				forged["sub"] = "admin"
				raw, _ := json.Marshal(forged)
				parts[1] = base64.RawURLEncoding.EncodeToString(raw)
				return strings.Join(parts, ".")
			},
			want: "signature does not verify",
		},
		{
			name: "missing kid",
			token: func() string {
				return mint(t, priv, map[string]any{"alg": "EdDSA"}, validPayload(now))
			},
			want: "no kid",
		},
		{
			name:  "not a JWS",
			token: func() string { return "not.a-token" },
			want:  "three-part",
		},
		{
			name:  "garbage header",
			token: func() string { return "!!!.!!!.!!!" },
			want:  "header",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestVerifier(t, pub, now)
			claims, err := v.Verify(context.Background(), tc.token())
			if err == nil {
				t.Fatalf("Verify accepted the token and returned %+v", claims)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestVerifyAcceptsAudienceArray(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	v := newTestVerifier(t, pub, now)

	payload := validPayload(now)
	payload["aud"] = []string{"someone-else", testAudience}

	if _, err := v.Verify(context.Background(), mint(t, priv, map[string]any{"alg": "EdDSA", "kid": testKid}, payload)); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyCachesJWKSAndRefetchesOnRotation(t *testing.T) {
	oldPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	newPub, newPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	published := map[string]ed25519.PublicKey{testKid: oldPub}
	var hits atomic.Int32
	server := jwksServer(t, published, &hits)

	now := time.Now()
	v := NewVerifier(server.URL, testIssuer, testAudience)
	v.now = func() time.Time { return now }

	// A token under the rotated-in kid: unknown, so the key set is fetched.
	rotated := mint(t, newPriv, map[string]any{"alg": "EdDSA", "kid": "rotated"}, validPayload(now))

	if _, err := v.Verify(context.Background(), rotated); err == nil {
		t.Fatal("expected failure before the new key was published")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("JWKS hits = %d, want 1", got)
	}

	// Still inside the refresh floor: an unknown kid must not cause a refetch.
	if _, err := v.Verify(context.Background(), rotated); err == nil {
		t.Fatal("expected failure")
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("JWKS hits = %d after a repeat unknown kid, want 1", got)
	}

	// The issuer rotates and the floor passes: the next attempt refetches.
	published["rotated"] = newPub
	now = now.Add(2 * minJWKSRefresh)

	if _, err := v.Verify(context.Background(), rotated); err != nil {
		t.Fatalf("Verify after rotation: %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("JWKS hits = %d, want 2", got)
	}
}

func TestVerifyRejectsOversizedToken(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	v := newTestVerifier(t, pub, time.Now())

	if _, err := v.Verify(context.Background(), strings.Repeat("a", maxTokenBytes+1)); err == nil {
		t.Fatal("expected an oversized token to be rejected")
	}
}

// Golden case: a real token and JWKS captured from Better Auth 1.6.25 with the
// jwt plugin, so a change in its token format fails here rather than in prod.
func TestVerifyAgainstCapturedBetterAuthToken(t *testing.T) {
	const capturedToken = "eyJhbGciOiJFZERTQSIsImtpZCI6IkhkRmI5b09ROTA5OGVPbVlHRHBMMFJiRTFwVVlaRG4wIn0.eyJpYXQiOjE3ODU2NzczNTksIm5hbWUiOiJHbyIsImVtYWlsIjoiZ28tMTc4NTY3NzM1ODcwNEBzeS50ZXN0IiwiZW1haWxWZXJpZmllZCI6ZmFsc2UsImltYWdlIjpudWxsLCJjcmVhdGVkQXQiOiIyMDI2LTA4LTAyVDEzOjI5OjE5LjIwNloiLCJ1cGRhdGVkQXQiOiIyMDI2LTA4LTAyVDEzOjI5OjE5LjIwNloiLCJpZCI6IlVzU2JmMTJaMmtPejRITmdhRGVVUEVNQW4wS0NydVpqIiwic3ViIjoiVXNTYmYxMloya096NEhOZ2FEZVVQRU1BbjBLQ3J1WmoiLCJleHAiOjE3ODU2NzgyNTksImlzcyI6Imh0dHA6Ly9sb2NhbGhvc3Q6MzAwMCIsImF1ZCI6InN3aXRjaHlhcmQtYmFja2VuZCJ9.fk2Xzzg_Ni33udHMtCDxv2J58VIv72-6tRatwkQeNHn7EYuuyckN6XxCenOlV4RSnBfk9HZMp-lLJSN252oCDQ"
	const capturedJWKS = `{"keys":[{"alg":"EdDSA","crv":"Ed25519","x":"1xE4UUrgAx2X01CJYEe6CgRV6ir63UGySJJjmRQD46g","kty":"OKP","kid":"HdFb9oOQ9098eOmYGDpL0RbE1pUYZDn0"}]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(capturedJWKS))
	}))
	defer server.Close()

	v := NewVerifier(server.URL, testIssuer, testAudience)
	// The captured token carries a real expiry, so pin the clock inside it.
	v.now = func() time.Time { return time.Unix(1785677400, 0) }

	claims, err := v.Verify(context.Background(), capturedToken)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "UsSbf12Z2kOz4HNgaDeUPEMAn0KCruZj" {
		t.Errorf("Subject = %q", claims.Subject)
	}
	if claims.Email != "go-1785677358704@sy.test" {
		t.Errorf("Email = %q", claims.Email)
	}

	// The same token must fail once its expiry has passed.
	v.now = func() time.Time { return time.Unix(1785678259, 0).Add(2 * clockSkew) }
	if _, err := v.Verify(context.Background(), capturedToken); err == nil {
		t.Error("expected the captured token to be rejected after expiry")
	}
}
