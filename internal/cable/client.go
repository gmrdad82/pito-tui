package cable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// Config wires one client to one conversation's JSON stream.
type Config struct {
	// InstanceURL is the http(s) base; the ws(s)://host/cable endpoint and
	// the Origin header both derive from it. Rails checks
	// allowed_request_origins on the handshake.
	InstanceURL string
	UUID        string
	// Jar carries the pito_session cookie onto the handshake.
	Jar http.CookieJar
	// PingTimeout is the liveness read deadline, refreshed on EVERY frame.
	// ActionCable sends application-level {"type":"ping"} JSON every ~3s —
	// not websocket control pings — so 6s (2 intervals) means dead.
	// Tests shrink it to keep reconnect cases fast.
	PingTimeout time.Duration
	// OnMessage receives append/replace broadcasts (already unwrapped).
	OnMessage func(StreamMessage)
	// OnState receives lifecycle transitions; err is non-nil on
	// StateDisconnected when there is something to say.
	OnState func(ConnState, error)
}

type Client struct {
	cfg   Config
	wsURL string
}

// New validates the config and derives the websocket URL.
func New(cfg Config) (*Client, error) {
	base, err := url.Parse(strings.TrimRight(cfg.InstanceURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("cable: invalid instance URL %q", cfg.InstanceURL)
	}
	scheme := "ws"
	if base.Scheme == "https" {
		scheme = "wss"
	}
	if cfg.PingTimeout <= 0 {
		cfg.PingTimeout = 6 * time.Second
	}
	if cfg.OnMessage == nil {
		cfg.OnMessage = func(StreamMessage) {}
	}
	if cfg.OnState == nil {
		cfg.OnState = func(ConnState, error) {}
	}
	return &Client{cfg: cfg, wsURL: scheme + "://" + base.Host + "/cable"}, nil
}

// Run connects, subscribes, and dispatches until ctx is cancelled or the
// server declares the session unauthorized (the app layer handles that —
// retrying an expired cookie would loop forever). Every other failure
// reconnects with jittered exponential backoff; the backoff resets only
// after a CONFIRMED subscription, so a server that accepts sockets but
// rejects subscribes can't hot-loop at the base delay.
func (c *Client) Run(ctx context.Context) error {
	attempt := 0
	for {
		c.cfg.OnState(StateConnecting, nil)
		confirmed, err := c.connectOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if confirmed {
			attempt = 0
		}
		c.cfg.OnState(StateDisconnected, err)
		if errors.Is(err, api.ErrUnauthorized) {
			return err
		}
		select {
		case <-time.After(backoff(attempt)):
			attempt++
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// connectOnce runs a single connection to failure. It returns whether the
// subscription was confirmed at any point (backoff-reset signal).
func (c *Client) connectOnce(ctx context.Context) (confirmed bool, err error) {
	dialer := websocket.Dialer{
		Jar:              c.cfg.Jar,
		Subprotocols:     []string{Subprotocol},
		HandshakeTimeout: 10 * time.Second,
	}
	header := http.Header{"Origin": {strings.TrimRight(c.cfg.InstanceURL, "/")}}
	conn, resp, err := dialer.DialContext(ctx, c.wsURL, header)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return false, api.ErrUnauthorized
		}
		return false, err
	}
	defer conn.Close()

	// Unblock the blocking read when the context is cancelled.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	ident := Identifier(c.cfg.UUID)
	refresh := func() { _ = conn.SetReadDeadline(time.Now().Add(c.cfg.PingTimeout)) }
	refresh()

	for {
		var f frame
		if err := conn.ReadJSON(&f); err != nil {
			return confirmed, err
		}
		refresh() // any frame proves the server is alive

		switch f.Type {
		case "welcome":
			if err := conn.WriteJSON(map[string]string{
				"command": "subscribe", "identifier": ident,
			}); err != nil {
				return confirmed, err
			}
		case "confirm_subscription":
			if f.Identifier == ident {
				confirmed = true
				c.cfg.OnState(StateConnected, nil)
			}
		case "reject_subscription":
			// The TuiChannel rejects unauthenticated sessions.
			return confirmed, api.ErrUnauthorized
		case "ping":
			// Deadline already refreshed; nothing else to do.
		case "disconnect":
			if f.Reason == "unauthorized" {
				return confirmed, api.ErrUnauthorized
			}
			return confirmed, fmt.Errorf("cable: server disconnect: %s", f.Reason)
		case "":
			// Broadcast frames carry no "type" — identifier + message.
			if f.Identifier != ident || len(f.Message) == 0 {
				continue
			}
			var sm StreamMessage
			if jsonErr := json.Unmarshal(f.Message, &sm); jsonErr == nil {
				c.cfg.OnMessage(sm)
			}
		default:
			// Unknown frame type: ignore. Novelty must never crash.
		}
	}
}
