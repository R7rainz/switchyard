// Package oauth handles provider authorization-code flows and stores the
// resulting token document through the credential service.
package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/R7rainz/switchyard/backend/internal/credential"
)

type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	Scope        string
}

type Service struct {
	providers   map[string]ProviderConfig
	credentials *credential.Service
	stateKey    []byte
	client      *http.Client
	now         func() time.Time
}

func NewService(providers map[string]ProviderConfig, stateKey []byte, credentials *credential.Service) *Service {
	return &Service{providers: providers, stateKey: stateKey, credentials: credentials, client: http.DefaultClient, now: time.Now}
}

type state struct {
	WorkspaceID string `json:"workspaceId"`
	Provider    string `json:"provider"`
	ExpiresAt   int64  `json:"expiresAt"`
}

type token struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
}

func (s *Service) Start(_ context.Context, workspaceID, provider, callbackURL string) (string, error) {
	config, ok := s.providers[provider]
	if !ok || config.ClientID == "" || config.AuthURL == "" || config.TokenURL == "" || len(s.stateKey) == 0 {
		return "", errors.New("oauth: provider is not configured")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("oauth: generating state: %w", err)
	}
	payload, err := json.Marshal(state{WorkspaceID: workspaceID, Provider: provider, ExpiresAt: s.now().Add(10 * time.Minute).Unix()})
	if err != nil {
		return "", fmt.Errorf("oauth: encoding state: %w", err)
	}
	// The nonce makes otherwise identical starts unlinkable; the signed payload
	// is what prevents a callback from changing its workspace or provider.
	payload = append(payload, nonce...)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := sign(s.stateKey, encoded)
	authorize, err := url.Parse(config.AuthURL)
	if err != nil || authorize.Scheme == "" || authorize.Host == "" {
		return "", errors.New("oauth: provider authorization URL is invalid")
	}
	query := authorize.Query()
	query.Set("client_id", config.ClientID)
	query.Set("redirect_uri", callbackURL)
	query.Set("response_type", "code")
	query.Set("state", encoded+"."+signature)
	if config.Scope != "" {
		query.Set("scope", config.Scope)
	}
	authorize.RawQuery = query.Encode()
	return authorize.String(), nil
}

func (s *Service) Callback(ctx context.Context, provider, code, signedState, callbackURL string) (string, error) {
	config, ok := s.providers[provider]
	if !ok || config.ClientID == "" || config.ClientSecret == "" || config.TokenURL == "" || len(s.stateKey) == 0 {
		return "", errors.New("oauth: provider is not configured")
	}
	parts := strings.Split(signedState, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(sign(s.stateKey, parts[0]))) {
		return "", errors.New("oauth: invalid state")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(payload) <= 16 {
		return "", errors.New("oauth: invalid state")
	}
	var claim state
	if err := json.Unmarshal(payload[:len(payload)-16], &claim); err != nil || claim.Provider != provider || claim.WorkspaceID == "" || claim.ExpiresAt < s.now().Unix() {
		return "", errors.New("oauth: invalid state")
	}
	if code == "" {
		return "", errors.New("oauth: authorization code is required")
	}
	if s.credentials == nil {
		return "", errors.New("oauth: credential service is not configured")
	}

	form := url.Values{"client_id": {config.ClientID}, "client_secret": {config.ClientSecret}, "code": {code}, "redirect_uri": {callbackURL}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("oauth: token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("oauth: token exchange: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("oauth: token exchange returned %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("oauth: reading token: %w", err)
	}
	var result token
	if err := json.Unmarshal(body, &result); err != nil || result.AccessToken == "" {
		return "", errors.New("oauth: provider returned no access token")
	}
	encodedToken, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("oauth: encoding token: %w", err)
	}
	if err := s.credentials.Put(ctx, claim.WorkspaceID, provider, "default", credential.Secret(encodedToken)); err != nil {
		return "", fmt.Errorf("oauth: storing token: %w", err)
	}
	return claim.WorkspaceID, nil
}

func sign(key []byte, value string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
