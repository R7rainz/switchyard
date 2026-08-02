package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Better Auth signs with EdDSA over Ed25519 and publishes an OKP JWKS. This
// verifier accepts nothing else: a fixed algorithm means a token claiming any
// other alg is rejected before a key is ever selected, so the usual JWT
// algorithm-confusion attacks have nowhere to land.
const algEdDSA = "EdDSA"

const (
	// A Better Auth token runs about 500 bytes. The cap keeps a hostile caller
	// from making us base64-decode something enormous.
	maxTokenBytes = 8 << 10
	maxJWKSBytes  = 1 << 20

	// Tolerance for clock drift between this process and the token issuer.
	clockSkew = time.Minute

	// An unknown kid triggers a JWKS refetch, since the issuer may have
	// rotated keys. This floor stops a stream of bogus kids from turning into
	// a stream of outbound requests.
	minJWKSRefresh = time.Minute
)

// Claims is the subset of the token payload the rest of Switchyard uses.
type Claims struct {
	Subject   string
	Email     string
	Name      string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// Verifier checks Better Auth JWTs against the issuer's published JWKS.
//
// It is safe for concurrent use and caches the key set in memory.
type Verifier struct {
	jwksURL  string
	issuer   string
	audience string

	httpClient *http.Client
	now        func() time.Time

	mu        sync.RWMutex
	keys      map[string]ed25519.PublicKey
	lastFetch time.Time

	// Held across the network fetch so a burst of requests produces one call.
	fetchMu sync.Mutex
}

// NewVerifier returns a Verifier for the given JWKS endpoint. Tokens must carry
// exactly this issuer and list this audience.
func NewVerifier(jwksURL, issuer, audience string) *Verifier {
	return &Verifier{
		jwksURL:    jwksURL,
		issuer:     issuer,
		audience:   audience,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		now:        time.Now,
	}
}

// Verify checks the token's signature, issuer, audience, and validity window,
// and returns its claims. Every failure is a reason to reject the request.
func (v *Verifier) Verify(ctx context.Context, token string) (*Claims, error) {
	if len(token) > maxTokenBytes {
		return nil, fmt.Errorf("auth: token exceeds %d bytes", maxTokenBytes)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("auth: token is not a three-part JWS")
	}

	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &header); err != nil {
		return nil, fmt.Errorf("auth: decoding token header: %w", err)
	}
	if header.Alg != algEdDSA {
		return nil, fmt.Errorf("auth: alg %q rejected, only %s is accepted", header.Alg, algEdDSA)
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("auth: decoding token signature: %w", err)
	}
	if len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("auth: signature is %d bytes, want %d", len(signature), ed25519.SignatureSize)
	}

	key, err := v.key(ctx, header.Kid)
	if err != nil {
		return nil, err
	}

	// Signature first: nothing below this line trusts the payload.
	if !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), signature) {
		return nil, errors.New("auth: signature does not verify")
	}

	var payload struct {
		Subject   string          `json:"sub"`
		Issuer    string          `json:"iss"`
		Audience  json.RawMessage `json:"aud"`
		Expires   int64           `json:"exp"`
		IssuedAt  int64           `json:"iat"`
		NotBefore int64           `json:"nbf"`
		Email     string          `json:"email"`
		Name      string          `json:"name"`
	}
	if err := decodeSegment(parts[1], &payload); err != nil {
		return nil, fmt.Errorf("auth: decoding token payload: %w", err)
	}

	if payload.Issuer != v.issuer {
		return nil, fmt.Errorf("auth: issuer %q does not match %q", payload.Issuer, v.issuer)
	}
	if err := matchAudience(payload.Audience, v.audience); err != nil {
		return nil, err
	}
	if payload.Subject == "" {
		return nil, errors.New("auth: token has no subject")
	}

	// An unexpiring token is not something we accept, so a missing exp fails.
	if payload.Expires == 0 {
		return nil, errors.New("auth: token has no expiry")
	}
	now := v.now()
	expires := time.Unix(payload.Expires, 0)
	if now.After(expires.Add(clockSkew)) {
		return nil, fmt.Errorf("auth: token expired at %s", expires.UTC().Format(time.RFC3339))
	}
	if payload.NotBefore != 0 {
		notBefore := time.Unix(payload.NotBefore, 0)
		if now.Add(clockSkew).Before(notBefore) {
			return nil, fmt.Errorf("auth: token not valid until %s", notBefore.UTC().Format(time.RFC3339))
		}
	}

	claims := &Claims{
		Subject:   payload.Subject,
		Email:     payload.Email,
		Name:      payload.Name,
		ExpiresAt: expires,
	}
	if payload.IssuedAt != 0 {
		claims.IssuedAt = time.Unix(payload.IssuedAt, 0)
	}
	return claims, nil
}

// key returns the public key for kid, refetching the JWKS if the kid is new.
func (v *Verifier) key(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	if kid == "" {
		return nil, errors.New("auth: token header has no kid")
	}

	v.mu.RLock()
	key, found := v.keys[kid]
	fetched := v.lastFetch
	v.mu.RUnlock()
	if found {
		return key, nil
	}
	if !fetched.IsZero() && v.now().Sub(fetched) < minJWKSRefresh {
		return nil, fmt.Errorf("auth: no key for kid %q", kid)
	}

	v.fetchMu.Lock()
	defer v.fetchMu.Unlock()

	// Another goroutine may have refreshed while we waited for the lock.
	v.mu.RLock()
	key, found = v.keys[kid]
	fetched = v.lastFetch
	v.mu.RUnlock()
	if found {
		return key, nil
	}
	if !fetched.IsZero() && v.now().Sub(fetched) < minJWKSRefresh {
		return nil, fmt.Errorf("auth: no key for kid %q", kid)
	}

	keys, err := v.fetchJWKS(ctx)
	if err != nil {
		return nil, err
	}

	v.mu.Lock()
	v.keys = keys
	v.lastFetch = v.now()
	v.mu.Unlock()

	key, found = keys[kid]
	if !found {
		return nil, fmt.Errorf("auth: no key for kid %q", kid)
	}
	return key, nil
}

func (v *Verifier) fetchJWKS(ctx context.Context) (map[string]ed25519.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: building JWKS request: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: fetching JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: JWKS endpoint returned %s", resp.Status)
	}

	var document struct {
		Keys []struct {
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			Alg string `json:"alg"`
			Kid string `json:"kid"`
			X   string `json:"x"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJWKSBytes)).Decode(&document); err != nil {
		return nil, fmt.Errorf("auth: decoding JWKS: %w", err)
	}

	keys := make(map[string]ed25519.PublicKey, len(document.Keys))
	for _, jwk := range document.Keys {
		// Skip rather than fail: an issuer may publish key types we do not use,
		// and one unusable entry should not blind us to the rest.
		if jwk.Kty != "OKP" || jwk.Crv != "Ed25519" || jwk.Kid == "" {
			continue
		}
		if jwk.Alg != "" && jwk.Alg != algEdDSA {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(jwk.X)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			continue
		}
		keys[jwk.Kid] = ed25519.PublicKey(raw)
	}

	if len(keys) == 0 {
		return nil, errors.New("auth: JWKS contained no usable Ed25519 keys")
	}
	return keys, nil
}

func decodeSegment(segment string, target any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

// matchAudience accepts the "aud" claim in either of its legal shapes: a single
// string, or an array of them.
func matchAudience(raw json.RawMessage, want string) error {
	if len(raw) == 0 {
		return errors.New("auth: token has no audience")
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single != want {
			return fmt.Errorf("auth: audience %q does not match %q", single, want)
		}
		return nil
	}

	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return errors.New("auth: audience is neither a string nor an array of strings")
	}
	for _, audience := range many {
		if audience == want {
			return nil
		}
	}
	return fmt.Errorf("auth: audience %v does not include %q", many, want)
}
