//go:build live

package cable

// Live diagnostic for the raw cable dial — excluded from CI. Run:
//
//	go test -tags live -run TestLiveDial -v ./internal/cable/
//
// Prints a precise timeline of the handshake and the first frames so edge
// weirdness (Cloudflare, tunnels) is distinguishable from client bugs.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestLiveDial(t *testing.T) {
	instance := os.Getenv("PITO_INSTANCE")
	if instance == "" {
		instance = "https://dev.pitomd.com"
	}
	host := instance[len("https://"):]

	// Read the session cookie the way the binary would.
	home, _ := os.UserHomeDir()
	raw, err := os.ReadFile(filepath.Join(home, ".config", "pito-tui", "cookies.json"))
	if err != nil {
		t.Fatalf("no cookie jar; run the app or TestLiveSmoke first: %v", err)
	}
	var stored []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		URL   string `json:"url"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	cookie := ""
	for _, c := range stored {
		if c.Name == "pito_session" && c.URL == instance+"/" {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("no pito_session cookie for the instance in the jar")
	}

	start := time.Now()
	stamp := func(format string, args ...any) {
		t.Logf("%7.2fs "+format, append([]any{time.Since(start).Seconds()}, args...)...)
	}

	dialer := websocket.Dialer{
		Subprotocols:     []string{Subprotocol},
		HandshakeTimeout: 10 * time.Second,
	}
	header := http.Header{
		"Origin": {instance},
		"Cookie": {"pito_session=" + cookie},
	}
	stamp("dialing wss://%s/cable", host)
	conn, resp, err := dialer.Dial("wss://"+host+"/cable", header)
	if err != nil {
		status := "no response"
		if resp != nil {
			status = resp.Status
		}
		stamp("dial FAILED: %v (response: %s)", err, status)
		t.FailNow()
	}
	defer conn.Close()
	stamp("dial OK: %s (subprotocol %q)", resp.Status, resp.Header.Get("Sec-Websocket-Protocol"))

	// Read frames for a while; send the subscribe after welcome.
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	for i := 0; i < 6; i++ {
		_, frame, err := conn.ReadMessage()
		if err != nil {
			stamp("read FAILED: %v", err)
			t.FailNow()
		}
		stamp("frame: %.120s", frame)
		var f struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(frame, &f)
		if f.Type == "welcome" {
			sub := map[string]string{"command": "subscribe", "identifier": Identifier(os.Getenv("PITO_CONV_UUID"))}
			if err := conn.WriteJSON(sub); err != nil {
				stamp("subscribe write FAILED: %v", err)
				t.FailNow()
			}
			stamp("subscribe sent (uuid %q)", os.Getenv("PITO_CONV_UUID"))
		}
		if f.Type == "confirm_subscription" {
			stamp("SUBSCRIPTION CONFIRMED")
			return
		}
		if f.Type == "reject_subscription" {
			stamp("SUBSCRIPTION REJECTED")
			t.FailNow()
		}
		_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	}
	t.Fatal("no confirm/reject within the frame budget")
}
