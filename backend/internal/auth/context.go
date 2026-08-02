package auth

import "context"

// contextKey is unexported so nothing outside this package can collide with it.
type contextKey struct{}

// NewContext returns a copy of ctx carrying the verified claims.
func NewContext(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, contextKey{}, claims)
}

// FromContext returns the claims established by the API layer's auth
// middleware. The second result is false on unauthenticated requests, so a
// caller that requires identity must check it rather than assume.
func FromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(contextKey{}).(*Claims)
	return claims, ok
}
