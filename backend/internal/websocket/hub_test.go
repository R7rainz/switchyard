package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// serveHub starts a real server carrying the same write timeout the API server
// uses. A hijacked connection keeps the deadline net/http set before the
// handler ran, so a test on a server without one proves nothing about
// production.
func serveHub(t *testing.T, hub *Hub, topic string, writeTimeout time.Duration) string {
	t.Helper()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = hub.Serve(w, r, topic)
	}))
	server.Config.WriteTimeout = writeTimeout
	server.Start()
	t.Cleanup(server.Close)

	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.Dial(t.Context(), url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

// waitForSubscriber blocks until the server side has registered the client.
// Dial returns once the handshake completes, which is before Serve has added
// the client to the topic.
func waitForSubscriber(t *testing.T, hub *Hub, topic string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.Subscribers(topic) == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("topic %q has %d subscribers, want %d", topic, hub.Subscribers(topic), want)
}

func readEvent(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	_, raw, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("event is not JSON: %s", raw)
	}
	return event
}

func TestPublishReachesASubscriber(t *testing.T) {
	hub := NewHub("")
	conn := dial(t, serveHub(t, hub, "run:1", 30*time.Second))
	waitForSubscriber(t, hub, "run:1", 1)

	hub.Publish("run:1", map[string]any{"status": "RUNNING"})

	if event := readEvent(t, conn); event["status"] != "RUNNING" {
		t.Fatalf("event = %v", event)
	}
}

// The whole point of a topic. A second run's events must not reach the first
// run's watcher.
func TestPublishDoesNotCrossTopics(t *testing.T) {
	hub := NewHub("")
	conn := dial(t, serveHub(t, hub, "run:1", 30*time.Second))
	waitForSubscriber(t, hub, "run:1", 1)

	hub.Publish("run:2", map[string]any{"status": "SOMEONE ELSE"})
	hub.Publish("run:1", map[string]any{"status": "MINE"})

	// If the topics leaked, the first message read here is the other run's.
	if event := readEvent(t, conn); event["status"] != "MINE" {
		t.Fatalf("received another topic's event: %v", event)
	}
}

// The API server sets WriteTimeout, and a stream has to outlive it.
//
// net/http clears the connection's deadlines when a handler hijacks, so this
// works — but that is the runtime's behaviour rather than ours, and if it ever
// changed the symptom would be every stream going silent a fixed time after it
// opened, with no error and no log. The harness sets a real WriteTimeout for
// that reason: on a server without one this test would pass while proving
// nothing.
func TestStreamOutlivesTheServersWriteTimeout(t *testing.T) {
	const writeTimeout = 300 * time.Millisecond

	hub := NewHub("")
	conn := dial(t, serveHub(t, hub, "run:1", writeTimeout))
	waitForSubscriber(t, hub, "run:1", 1)

	// A run that sits waiting on a slow HTTP node does exactly this.
	time.Sleep(2 * writeTimeout)

	hub.Publish("run:1", map[string]any{"status": "SUCCEEDED"})

	if event := readEvent(t, conn); event["status"] != "SUCCEEDED" {
		t.Fatalf("event = %v", event)
	}
}

// A browser that has stopped reading must not be able to hold the engine up.
// Publish is what the engine calls while running a workflow.
func TestPublishNeverBlocksOnASlowClient(t *testing.T) {
	hub := NewHub("")
	url := serveHub(t, hub, "run:1", 30*time.Second)
	dial(t, url)
	waitForSubscriber(t, hub, "run:1", 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Far more than one client can buffer, and nothing is reading.
		for range sendBuffer * 4 {
			hub.Publish("run:1", map[string]any{"status": "RUNNING"})
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish blocked on a client that stopped reading")
	}
}

// A client whose buffer is full is dropped rather than waited for.
//
// Tested against the hub directly rather than through a socket: a real client
// that stops calling Read still has a kernel receive buffer, which swallows
// enough small messages that the send channel never fills. Reproducing that end
// to end would mean pushing megabytes to prove a branch that is three lines.
func TestAClientThatFallsBehindIsDropped(t *testing.T) {
	hub := NewHub("")

	// Unbuffered, with nothing serving it: the next send has nowhere to go.
	stuck := &client{send: make(chan []byte), closed: make(chan struct{})}
	hub.add("run:1", stuck)

	hub.Publish("run:1", map[string]any{"status": "RUNNING"})

	select {
	case <-stuck.closed:
	default:
		t.Fatal("a client that could not accept an event was kept")
	}
}

// Publishing to a client that was already dropped must not panic or block.
func TestPublishToADroppedClientIsHarmless(t *testing.T) {
	hub := NewHub("")
	gone := &client{send: make(chan []byte), closed: make(chan struct{})}
	gone.close()
	hub.add("run:1", gone)

	hub.Publish("run:1", map[string]any{"status": "RUNNING"})
	hub.Publish("run:1", map[string]any{"status": "SUCCEEDED"})
}

// A topic nobody watches must not stay in the map, or it grows by one entry per
// execution ever opened.
func TestTopicsAreForgottenWhenEmpty(t *testing.T) {
	hub := NewHub("")
	conn := dial(t, serveHub(t, hub, "run:1", 30*time.Second))
	waitForSubscriber(t, hub, "run:1", 1)

	_ = conn.Close(websocket.StatusNormalClosure, "done")
	waitForSubscriber(t, hub, "run:1", 0)

	hub.mu.RLock()
	defer hub.mu.RUnlock()
	if _, still := hub.topics["run:1"]; still {
		t.Fatal("the topic outlived its last subscriber")
	}
}

// Publishing while clients come and go is the normal state of a busy server.
func TestPublishIsSafeUnderChurn(t *testing.T) {
	hub := NewHub("")
	url := serveHub(t, hub, "run:1", 30*time.Second)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				hub.Publish("run:1", map[string]any{"status": "RUNNING"})
			}
		})
	}
	for range 4 {
		wg.Go(func() {
			conn, _, err := websocket.Dial(t.Context(), url, nil)
			if err != nil {
				return
			}
			_ = conn.CloseNow()
		})
	}
	wg.Wait()
}

func TestStripScheme(t *testing.T) {
	cases := map[string]string{
		"http://localhost:3007":   "localhost:3007",
		"https://app.example.com": "app.example.com",
		"localhost:3007":          "localhost:3007",
	}
	for input, want := range cases {
		if got := stripScheme(input); got != want {
			t.Errorf("stripScheme(%q) = %q, want %q", input, got, want)
		}
	}
}

// A client is subscribed by the time Dial returns.
//
// Subscribing after the handshake leaves a window where the client believes it
// is connected and the hub has never heard of it — anything published in that
// window goes nowhere. That is precisely the gap the connect-before-you-fetch
// ordering exists to close, so leaving it open here would make that contract a
// lie.
func TestSubscribedBeforeTheHandshakeCompletes(t *testing.T) {
	hub := NewHub("")
	url := serveHub(t, hub, "run:1", 30*time.Second)

	conn := dial(t, url)

	// No polling: if this needs a retry loop, the window is real.
	if watching := hub.Subscribers("run:1"); watching != 1 {
		t.Fatalf("Subscribers = %d immediately after Dial, want 1", watching)
	}

	hub.Publish("run:1", map[string]any{"status": "RUNNING"})
	if event := readEvent(t, conn); event["status"] != "RUNNING" {
		t.Fatalf("event = %v", event)
	}
}
