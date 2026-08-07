package oauth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/R7rainz/switchyard/backend/internal/credential"
)

func TestAuthorizationCodeStoresEncryptedToken(t *testing.T) {
	key := make([]byte, credential.KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	ring, err := credential.NewKeyring(1, map[int][]byte{1: key})
	if err != nil {
		t.Fatal(err)
	}
	credentials := credential.NewService(credential.NewMemoryStore(), ring)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("code") != "code-1" {
			t.Error("token endpoint did not receive the code")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-1","token_type":"bearer","refresh_token":"rt-1"}`))
	}))
	defer server.Close()

	service := NewService(map[string]ProviderConfig{"github": {
		ClientID: "client", ClientSecret: "secret", AuthURL: server.URL + "/authorize", TokenURL: server.URL + "/token",
	}}, []byte("01234567890123456789012345678901"), credentials)
	callback := "https://switchyard.example/api/v1/oauth/callback/github"
	authorize, err := service.Start(context.Background(), "ws-1", "github", callback)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	parsed, err := url.Parse(authorize)
	if err != nil || parsed.Query().Get("state") == "" {
		t.Fatalf("authorize URL = %q", authorize)
	}
	workspaceID, err := service.Callback(context.Background(), "github", "code-1", parsed.Query().Get("state"), callback)
	if err != nil || workspaceID != "ws-1" {
		t.Fatalf("Callback = %q, %v", workspaceID, err)
	}
	secret, err := credentials.Get(context.Background(), "ws-1", "github", "default")
	if err != nil {
		t.Fatalf("stored token: %v", err)
	}
	var token map[string]string
	if json.Unmarshal(secret, &token) != nil || token["access_token"] != "at-1" {
		t.Fatalf("stored token = %q", secret)
	}
}

func TestCallbackRejectsTamperedState(t *testing.T) {
	service := NewService(map[string]ProviderConfig{"github": {ClientID: "id", ClientSecret: "secret", TokenURL: "https://example.test/token"}},
		[]byte("01234567890123456789012345678901"), nil)
	if _, err := service.Callback(context.Background(), "github", "code", "bad.state", "https://callback"); err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("Callback error = %v", err)
	}
}
