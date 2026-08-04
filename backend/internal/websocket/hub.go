package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Hub fans events out to whoever is watching a topic.
//
// It holds no state worth reading: a client that misses a message re-reads the
// execution over REST, which is the record. That is what lets a slow client be
// dropped rather than waited for.
type Hub struct {
	mu     sync.RWMutex
	topics map[string]map[*client]struct{}

	// origins is what Accept will allow. The frontend is a different origin
	// from this API — different port is enough — so the browser's Origin has
	// to be allowed explicitly, exactly as it does for CORS.
	origins []string
}

func NewHub(appURL string) *Hub {
	origins := []string{}
	if appURL != "" {
		origins = append(origins, stripScheme(appURL))
	}
	return &Hub{topics: make(map[string]map[*client]struct{}), origins: origins}
}

// sendBuffer is how far behind one client may fall before it is dropped.
//
// A run of a few hundred nodes emits a couple of events each, so this absorbs a
// browser that stalls for a moment without holding the engine up.
const sendBuffer = 64

type client struct {
	send   chan []byte
	closed chan struct{}
	once   sync.Once
}

func (c *client) close() { c.once.Do(func() { close(c.closed) }) }

// Publish sends event to every client watching topic.
//
// It never blocks and never fails: the engine calls this while running a
// workflow, and a browser that has stopped reading must not be able to hold a
// run up. A client that cannot keep up is dropped instead, and reconnects to a
// state it re-reads over REST.
func (h *Hub) Publish(topic string, event any) {
	message, err := json.Marshal(event)
	if err != nil {
		// Nothing useful to do here — there is no caller waiting on this and no
		// logger in a hub. An event that will not marshal is a bug in the
		// struct, which its own package's tests are the place to catch.
		return
	}

	h.mu.RLock()
	clients := make([]*client, 0, len(h.topics[topic]))
	for c := range h.topics[topic] {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- message:
		case <-c.closed:
		default:
			// Too far behind. Dropping is the honest outcome: the alternative
			// is blocking the engine on a browser somebody closed the lid on.
			c.close()
		}
	}
}

// Subscribers reports how many clients are watching a topic.
func (h *Hub) Subscribers(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.topics[topic])
}

func (h *Hub) add(topic string, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.topics[topic] == nil {
		h.topics[topic] = make(map[*client]struct{})
	}
	h.topics[topic][c] = struct{}{}
}

func (h *Hub) remove(topic string, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.topics[topic], c)
	// The last client leaving takes the topic with it, or the map grows by one
	// entry per execution ever watched and never shrinks.
	if len(h.topics[topic]) == 0 {
		delete(h.topics, topic)
	}
}

const (
	// pingEvery keeps idle connections alive. A run can sit waiting on a slow
	// HTTP node for minutes, and proxies close connections that say nothing.
	pingEvery = 30 * time.Second

	// writeWait bounds one write, so a client that has stopped reading its
	// socket cannot pin the goroutine serving it.
	writeWait = 10 * time.Second
)

// bearerSubprotocol is what the browser offers to carry its token. The
// negotiated value is echoed back so the handshake completes; nothing reads it
// here, because RequireAuth has already verified the token by this point.
const bearerSubprotocol = "bearer"

// Serve upgrades the request and streams topic to it until the client goes
// away.
//
// Auth is not this function's job. It runs behind the same RequireAuth and
// RequirePermission the REST routes use, and the caller has already confirmed
// the subscription is one this user may have.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, topic string) error {
	// Subscribed before the handshake, not after.
	//
	// Accept writes the 101 that releases the client, so subscribing afterwards
	// races it: the client believes it is connected while the hub has never
	// heard of it, and anything published in that window goes nowhere. That is
	// the exact gap connect-before-you-fetch exists to close, so leaving it open
	// would make the contract a lie. Events arriving before the loop below runs
	// wait in the buffer.
	c := &client{send: make(chan []byte, sendBuffer), closed: make(chan struct{})}
	h.add(topic, c)
	defer h.remove(topic, c)

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:   []string{bearerSubprotocol},
		OriginPatterns: h.origins,
	})
	if err != nil {
		// Accept has already written a response by this point.
		return err
	}
	defer conn.CloseNow()

	// Nothing is expected from the client, but the connection still has to be
	// read for close and pong frames to be processed. CloseRead does that and
	// hands back a context that ends when the peer goes away.
	ctx := conn.CloseRead(r.Context())

	ping := time.NewTicker(pingEvery)
	defer ping.Stop()

	for {
		select {
		case message := <-c.send:
			if err := writeWithin(ctx, conn, message); err != nil {
				return nil
			}

		case <-ping.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeWait)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return nil
			}

		case <-c.closed:
			// Dropped for falling behind. Saying so beats a silent hang: the
			// client learns to reconnect and re-read rather than sitting on a
			// socket that has quietly stopped carrying events.
			_ = conn.Close(websocket.StatusPolicyViolation, "too slow")
			return nil

		case <-ctx.Done():
			return nil
		}
	}
}

func writeWithin(ctx context.Context, conn *websocket.Conn, message []byte) error {
	ctx, cancel := context.WithTimeout(ctx, writeWait)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, message)
}

// stripScheme turns a URL into the host pattern OriginPatterns wants, which
// matches against the Origin header's host rather than the whole URL.
func stripScheme(raw string) string {
	if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return raw
}
