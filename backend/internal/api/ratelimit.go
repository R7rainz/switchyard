package api

import (
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// How much generation a workspace gets by default. Generous enough that a
// person drafting workflows never notices, small enough that a loop does.
const (
	defaultGeneratePerHour = 30
	defaultGenerateBurst   = 5
)

// Limiter is a token bucket per key.
//
// It exists to stop one workspace draining a budget. Generation spends a real
// API key on every call and the endpoint had no limit at all, which is
// survivable only while the key belongs to the caller — the moment the platform
// fronts one, an unmetered endpoint is an open tap on somebody else's money.
//
// A bucket rather than a fixed window: the honest shape of this is "a few in a
// row is fine, a thousand an hour is not". A fixed window would allow twice the
// rate across a boundary and refuse a second click a minute after the first.
//
// In memory, and so per process. That matches how the rest of this binary
// thinks about liveness — Cancel only reaches runs in this process for the same
// reason — and one binary is the whole deployment. A second instance would
// double the effective limit, which is the point at which this belongs in
// Postgres or Redis rather than a map.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket

	// perSecond is the refill rate; burst is how many can be spent at once.
	perSecond float64
	burst     float64

	now       func() time.Time
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	seen   time.Time
}

// NewLimiter allows perHour requests per key over time, with burst of them
// available at once.
func NewLimiter(perHour, burst int) *Limiter {
	if perHour < 1 {
		perHour = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &Limiter{
		buckets:   make(map[string]*bucket),
		perSecond: float64(perHour) / 3600,
		burst:     float64(burst),
		now:       time.Now,
	}
}

// Allow spends a token for key, and reports how long to wait when it cannot.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, known := l.buckets[key]
	if !known {
		// A key arrives with a full bucket, so a workspace's first request is
		// never the one that waits.
		b = &bucket{tokens: l.burst}
		l.buckets[key] = b
	} else {
		b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.seen).Seconds()*l.perSecond)
	}
	b.seen = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	// Long enough for one token to exist, rounded up: a Retry-After that is a
	// fraction short sends the caller back to be refused again.
	wait := time.Duration((1 - b.tokens) / l.perSecond * float64(time.Second))
	return false, wait.Round(time.Second) + time.Second
}

// sweep drops buckets nobody has touched for long enough that they would have
// refilled anyway. Without it the map grows by one entry per workspace ever
// seen and never shrinks.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now

	// A bucket idle for this long is indistinguishable from a new one.
	idle := time.Duration(l.burst/l.perSecond) * time.Second
	for key, b := range l.buckets {
		if now.Sub(b.seen) > idle {
			delete(l.buckets, key)
		}
	}
}

// RateLimit refuses a request when its workspace has spent its allowance.
//
// Keyed on the workspace rather than the user, because the budget being
// protected is the workspace's: its stored key, and later its plan. Members of
// one workspace share it, which is the same thing as sharing the key.
func RateLimit(limiter *Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			workspaceID := chi.URLParam(r, "workspaceID")
			if workspaceID == "" {
				// Mounted somewhere with no workspace to key on. Fail closed,
				// exactly as RequirePermission does — a limiter that silently
				// stops limiting is worse than one that is obviously wrong.
				writeError(w, r, errNoWorkspace)
				return
			}

			if ok, retryAfter := limiter.Allow(workspaceID); !ok {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
				writeJSON(w, http.StatusTooManyRequests, map[string]any{
					"error": fmt.Sprintf(
						"this workspace has made too many generation requests; try again in %s",
						retryAfter.Round(time.Second)),
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
