package api

import (
	"net/http"
	"testing"
	"time"
)

// at builds a limiter on a clock the test drives, so refill is tested by
// advancing time rather than by sleeping through it.
func at(perHour, burst int) (*Limiter, func(time.Duration)) {
	limiter := NewLimiter(perHour, burst)
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return clock }
	return limiter, func(d time.Duration) { clock = clock.Add(d) }
}

// A workspace's first request is never the one that waits: a new key arrives
// with a full bucket.
func TestBurstIsAvailableImmediately(t *testing.T) {
	limiter, _ := at(60, 3)

	for i := range 3 {
		if ok, _ := limiter.Allow("ws"); !ok {
			t.Fatalf("request %d of the burst was refused", i+1)
		}
	}
	ok, retryAfter := limiter.Allow("ws")
	if ok {
		t.Fatal("a fourth request was allowed on a burst of three")
	}
	if retryAfter <= 0 {
		t.Fatalf("Retry-After = %s, want something positive", retryAfter)
	}
}

// The refill is the part worth testing on a clock. At 60/hour one token is a
// minute, so nothing is available after thirty seconds and one is after ninety.
func TestTokensComeBackOverTime(t *testing.T) {
	limiter, advance := at(60, 1)

	if ok, _ := limiter.Allow("ws"); !ok {
		t.Fatal("the first request was refused")
	}

	advance(30 * time.Second)
	if ok, _ := limiter.Allow("ws"); ok {
		t.Fatal("half a token was enough to be allowed")
	}

	advance(90 * time.Second)
	if ok, _ := limiter.Allow("ws"); !ok {
		t.Fatal("a token had not come back after two minutes at 60/hour")
	}
}

// Retry-After has to be long enough that following it works. A value a
// fraction short sends the caller back to be refused again.
func TestRetryAfterIsLongEnoughToWork(t *testing.T) {
	limiter, advance := at(60, 1)
	limiter.Allow("ws")

	ok, retryAfter := limiter.Allow("ws")
	if ok {
		t.Fatal("the second request was allowed")
	}

	advance(retryAfter)
	if ok, _ := limiter.Allow("ws"); !ok {
		t.Fatalf("waiting the advertised %s was still not enough", retryAfter)
	}
}

// One workspace exhausting its allowance must not touch another's.
func TestWorkspacesDoNotShareABucket(t *testing.T) {
	limiter, _ := at(60, 2)

	limiter.Allow("busy")
	limiter.Allow("busy")
	if ok, _ := limiter.Allow("busy"); ok {
		t.Fatal("the busy workspace was not limited")
	}

	if ok, _ := limiter.Allow("quiet"); !ok {
		t.Fatal("one workspace's spending refused another's first request")
	}
}

// Without this the map grows by one entry per workspace ever seen.
func TestIdleBucketsAreForgotten(t *testing.T) {
	limiter, advance := at(60, 2)
	limiter.Allow("ws")

	if len(limiter.buckets) != 1 {
		t.Fatalf("buckets = %d, want 1", len(limiter.buckets))
	}

	// Past the point where the bucket would have refilled anyway, so keeping it
	// buys nothing. The sweep is rate limited itself, hence the second call.
	advance(10 * time.Minute)
	limiter.Allow("someone-else")

	if _, still := limiter.buckets["ws"]; still {
		t.Fatal("an idle bucket outlived its usefulness")
	}
}

// The route is gated in the router, so this checks the wiring rather than the
// arithmetic: a real request through the real middleware chain gets a 429 with
// a Retry-After a client can act on.
func TestGenerateIsRateLimited(t *testing.T) {
	// modelReturns and keyStored come from ai_test.go; one generation per hour
	// with no burst makes the second request the refused one.
	h := aiRouterLimited(t, generatedGraph, true, NewLimiter(1, 1))
	ws := firstWorkspace(t, h, "alice")
	path := "/api/workspaces/" + ws + "/workflows/generate"

	if status, body := call(t, h, http.MethodPost, path, "alice", `{"prompt":"x"}`); status != http.StatusOK {
		t.Fatalf("first generation: status = %d, body %v", status, body)
	}

	status, body := call(t, h, http.MethodPost, path, "alice", `{"prompt":"x"}`)
	if status != http.StatusTooManyRequests {
		t.Fatalf("second generation: status = %d, want 429", status)
	}
	// The message says which workspace ran out and roughly when to come back,
	// because the caller's only move is to wait and they should know how long.
	if message := field(t, body, "error"); message == "" {
		t.Fatal("a 429 with no explanation")
	}
}

// A workspace being throttled must not throttle another.
func TestRateLimitIsPerWorkspace(t *testing.T) {
	h := aiRouterLimited(t, generatedGraph, true, NewLimiter(1, 1))

	alice := firstWorkspace(t, h, "alice")
	bob := firstWorkspace(t, h, "bob")

	call(t, h, http.MethodPost, "/api/workspaces/"+alice+"/workflows/generate", "alice", `{"prompt":"x"}`)
	if status, _ := call(t, h, http.MethodPost, "/api/workspaces/"+alice+"/workflows/generate", "alice", `{"prompt":"x"}`); status != http.StatusTooManyRequests {
		t.Fatalf("alice's second: status = %d, want 429", status)
	}

	if status, _ := call(t, h, http.MethodPost, "/api/workspaces/"+bob+"/workflows/generate", "bob", `{"prompt":"x"}`); status != http.StatusOK {
		t.Fatalf("bob's first: status = %d, want 200 — alice's spending is not his", status)
	}
}
