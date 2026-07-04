package cable

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// scriptServer runs one script per incoming connection (connection N gets
// scripts[N]; extras reuse the last script). Each script is a sequence of
// steps executed against the upgraded conn.
type scriptServer struct {
	t       *testing.T
	srv     *httptest.Server
	scripts [][]step
	conns   atomic.Int32
	// checkReq inspects the raw handshake request before upgrading.
	checkReq func(*http.Request)
}

type step func(t *testing.T, conn *websocket.Conn)

func send(raw string) step {
	return func(t *testing.T, conn *websocket.Conn) {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(raw)); err != nil {
			t.Logf("script send: %v", err)
		}
	}
}

// expectSubscribe reads one frame and asserts it is the byte-exact
// subscribe command for uuid.
func expectSubscribe(uuid string) step {
	return func(t *testing.T, conn *websocket.Conn) {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("script expect: %v", err)
			return
		}
		want := `{"command":"subscribe","identifier":"{\"channel\":\"Pito::JsonChannel\",\"uuid\":\"` + uuid + `\"}"}`
		var got, expected any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("subscribe frame not JSON: %s", raw)
		}
		if err := json.Unmarshal([]byte(want), &expected); err != nil {
			t.Fatal(err)
		}
		gotRaw, _ := json.Marshal(got)
		wantRaw, _ := json.Marshal(expected)
		if string(gotRaw) != string(wantRaw) {
			t.Errorf("subscribe frame:\n got %s\nwant %s", raw, want)
		}
	}
}

func hold(d time.Duration) step {
	return func(t *testing.T, conn *websocket.Conn) { time.Sleep(d) }
}

func newScriptServer(t *testing.T, scripts ...[]step) *scriptServer {
	t.Helper()
	s := &scriptServer{t: t, scripts: scripts}
	upgrader := websocket.Upgrader{
		Subprotocols: []string{Subprotocol},
		CheckOrigin:  func(*http.Request) bool { return true },
	}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.checkReq != nil {
			s.checkReq(r)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		n := int(s.conns.Add(1)) - 1
		if n >= len(s.scripts) {
			n = len(s.scripts) - 1
		}
		for _, st := range s.scripts[n] {
			st(t, conn)
		}
		// Keep the conn open until the client walks away.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// recorder collects callback traffic race-safely.
type recorder struct {
	messages chan StreamMessage
	states   chan ConnState
}

func newRecorder() *recorder {
	return &recorder{
		messages: make(chan StreamMessage, 32),
		states:   make(chan ConnState, 32),
	}
}

func (r *recorder) config(srvURL, uuid string, pingTimeout time.Duration) Config {
	return Config{
		InstanceURL: srvURL,
		UUID:        uuid,
		PingTimeout: pingTimeout,
		OnMessage:   func(m StreamMessage) { r.messages <- m },
		OnState:     func(s ConnState, _ error) { r.states <- s },
	}
}

func (r *recorder) waitState(t *testing.T, want ConnState) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case got := <-r.states:
			if got == want {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for state %v", want)
		}
	}
}

func (r *recorder) waitMessage(t *testing.T) StreamMessage {
	t.Helper()
	select {
	case m := <-r.messages:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a stream message")
		return StreamMessage{}
	}
}

const testUUID = "u1"

func broadcast(msgType string, id, turnID int64, kind string) string {
	return `{"identifier":"{\"channel\":\"Pito::JsonChannel\",\"uuid\":\"u1\"}",` +
		`"message":{"type":"` + msgType + `","event":{"id":` + itoa(id) +
		`,"turn_id":` + itoa(turnID) + `,"kind":"` + kind +
		`","payload":{"text":"hi"},"created_at":"2026-07-04T12:00:00Z"}}}`
}

func itoa(n int64) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

func runClient(t *testing.T, cfg Config) (context.CancelFunc, chan error) {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	stopped := make(chan struct{})
	go func() {
		done <- c.Run(ctx)
		close(stopped)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("cable Run did not stop on cancel")
		}
	})
	return cancel, done
}

func TestHappyPathSubscribeAndDispatch(t *testing.T) {
	srv := newScriptServer(t, []step{
		send(`{"type":"welcome"}`),
		expectSubscribe(testUUID),
		send(`{"type":"confirm_subscription","identifier":"{\"channel\":\"Pito::JsonChannel\",\"uuid\":\"u1\"}"}`),
		send(broadcast(TypeEventAppend, 42, 7, "system")),
		send(broadcast(TypeEventReplace, 42, 7, "system")),
	})
	rec := newRecorder()
	runClient(t, rec.config(srv.srv.URL, testUUID, time.Second))

	rec.waitState(t, StateConnected)
	first := rec.waitMessage(t)
	if first.Type != TypeEventAppend || first.Event.ID != 42 || first.Event.Kind != "system" {
		t.Errorf("first message = %+v", first)
	}
	second := rec.waitMessage(t)
	if second.Type != TypeEventReplace {
		t.Errorf("second message type = %q", second.Type)
	}
}

func TestPingsKeepTheConnectionAlive(t *testing.T) {
	pings := make([]step, 0, 8)
	for range 8 {
		pings = append(pings, send(`{"type":"ping","message":1751623456}`), hold(100*time.Millisecond))
	}
	srv := newScriptServer(t, append([]step{
		send(`{"type":"welcome"}`),
		expectSubscribe(testUUID),
		send(`{"type":"confirm_subscription","identifier":"{\"channel\":\"Pito::JsonChannel\",\"uuid\":\"u1\"}"}`),
	}, pings...))
	rec := newRecorder()
	runClient(t, rec.config(srv.srv.URL, testUUID, 300*time.Millisecond))

	rec.waitState(t, StateConnected)
	// 8 pings × 100ms = 800ms of traffic against a 300ms deadline: any
	// missed refresh would disconnect within the window.
	select {
	case s := <-rec.states:
		t.Errorf("state changed to %v during ping traffic", s)
	case <-time.After(700 * time.Millisecond):
	}
	if got := srv.conns.Load(); got != 1 {
		t.Errorf("connections = %d, want 1 (no reconnect)", got)
	}
}

func TestSilenceTriggersReconnectAndResubscribe(t *testing.T) {
	handshake := []step{
		send(`{"type":"welcome"}`),
		expectSubscribe(testUUID),
		send(`{"type":"confirm_subscription","identifier":"{\"channel\":\"Pito::JsonChannel\",\"uuid\":\"u1\"}"}`),
	}
	// First connection goes silent after confirming; the read deadline
	// must kill it and the client must dial again and re-subscribe.
	srv := newScriptServer(t,
		append(append([]step{}, handshake...), hold(2*time.Second)),
		handshake,
	)
	rec := newRecorder()
	runClient(t, rec.config(srv.srv.URL, testUUID, 150*time.Millisecond))

	rec.waitState(t, StateConnected)
	rec.waitState(t, StateDisconnected)
	rec.waitState(t, StateConnected)
	if got := srv.conns.Load(); got < 2 {
		t.Errorf("connections = %d, want >= 2", got)
	}
}

func TestRejectSubscriptionReturnsUnauthorized(t *testing.T) {
	srv := newScriptServer(t, []step{
		send(`{"type":"welcome"}`),
		expectSubscribe(testUUID),
		send(`{"type":"reject_subscription","identifier":"{\"channel\":\"Pito::JsonChannel\",\"uuid\":\"u1\"}"}`),
	})
	rec := newRecorder()
	_, done := runClient(t, rec.config(srv.srv.URL, testUUID, time.Second))

	select {
	case err := <-done:
		if !errors.Is(err, api.ErrUnauthorized) {
			t.Errorf("Run = %v, want ErrUnauthorized", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return on reject_subscription")
	}
}

func TestServerDisconnectUnauthorizedStopsForGood(t *testing.T) {
	srv := newScriptServer(t, []step{
		send(`{"type":"welcome"}`),
		expectSubscribe(testUUID),
		send(`{"type":"disconnect","reason":"unauthorized","reconnect":false}`),
	})
	rec := newRecorder()
	_, done := runClient(t, rec.config(srv.srv.URL, testUUID, time.Second))

	select {
	case err := <-done:
		if !errors.Is(err, api.ErrUnauthorized) {
			t.Errorf("Run = %v, want ErrUnauthorized", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return on disconnect:unauthorized")
	}
}

func TestHandshakeCarriesCookieOriginAndSubprotocol(t *testing.T) {
	srv := newScriptServer(t, []step{
		send(`{"type":"welcome"}`),
		expectSubscribe(testUUID),
		send(`{"type":"confirm_subscription","identifier":"{\"channel\":\"Pito::JsonChannel\",\"uuid\":\"u1\"}"}`),
	})
	srv.checkReq = func(r *http.Request) {
		if c, err := r.Cookie("pito_session"); err != nil || c.Value != "s3cr3t" {
			t.Error("handshake missing the session cookie")
		}
		if got := r.Header.Get("Origin"); got != srv.srv.URL {
			t.Errorf("Origin = %q, want %q", got, srv.srv.URL)
		}
		if got := r.Header.Get("Sec-WebSocket-Protocol"); got != Subprotocol {
			t.Errorf("subprotocol = %q", got)
		}
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(srv.srv.URL)
	jar.SetCookies(u, []*http.Cookie{{Name: "pito_session", Value: "s3cr3t"}})

	rec := newRecorder()
	cfg := rec.config(srv.srv.URL, testUUID, time.Second)
	cfg.Jar = jar
	runClient(t, cfg)
	// Wait for the CONFIRMED subscription: ending on "connecting" races
	// the teardown against the server script still reading the subscribe
	// frame (flaked on CI runners, never locally).
	rec.waitState(t, StateConnected)
}

func TestUnknownFramesAndForeignBroadcastsIgnored(t *testing.T) {
	srv := newScriptServer(t, []step{
		send(`{"type":"welcome"}`),
		expectSubscribe(testUUID),
		send(`{"type":"confirm_subscription","identifier":"{\"channel\":\"Pito::JsonChannel\",\"uuid\":\"u1\"}"}`),
		send(`{"type":"hologram","message":42}`),
		send(`{"identifier":"{\"channel\":\"Pito::JsonChannel\",\"uuid\":\"OTHER\"}","message":{"type":"event.append","event":{"id":1}}}`),
		send(broadcast(TypeEventAppend, 99, 1, "system")),
	})
	rec := newRecorder()
	runClient(t, rec.config(srv.srv.URL, testUUID, time.Second))

	// Only the last, correctly-addressed broadcast may arrive.
	if got := rec.waitMessage(t); got.Event.ID != 99 {
		t.Errorf("message = %+v, want event 99", got)
	}
}

func TestBackoffShape(t *testing.T) {
	if d := backoff(0); d < 400*time.Millisecond || d > 600*time.Millisecond {
		t.Errorf("backoff(0) = %v, want 500ms ±20%%", d)
	}
	for attempt := range 20 {
		if d := backoff(attempt); d > 36*time.Second {
			t.Errorf("backoff(%d) = %v exceeds the 30s cap (+jitter)", attempt, d)
		}
	}
	if backoff(10) < 20*time.Second {
		t.Errorf("backoff(10) = %v, want near the cap", backoff(10))
	}
}
