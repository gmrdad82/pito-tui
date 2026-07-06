//go:build live

package app

// Raw-cable probe for the conversation.update message (tui-needs.md item
// 1): after a send, the JSON stream must carry context + notifications
// on the same tick as the web meter refresh.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
	"github.com/gmrdad82/pito-tui/internal/config"
)

func TestConversationUpdateOnCable(t *testing.T) {
	instance := os.Getenv("PITO_INSTANCE")
	if instance == "" {
		instance = "https://dev.pitomd.com"
	}
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	client, err := api.New(instance, filepath.Join(dir, "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	res, err := client.SendMessage(ctx, "", "greet", 1000)
	if err != nil || res.CreatedUUID == "" {
		t.Fatalf("create: %v %+v", err, res)
	}
	uuid := res.CreatedUUID

	wsURL := "wss://" + strings.TrimPrefix(instance, "https://") + "/cable"
	dialer := websocket.Dialer{
		Subprotocols: []string{cable.Subprotocol},
		Jar:          client.Jar(),
	}
	conn, _, err := dialer.Dial(wsURL, http.Header{"Origin": {instance}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	ident, err := json.Marshal(map[string]string{"channel": cable.ChannelName, "uuid": uuid})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(map[string]any{"command": "subscribe", "identifier": string(ident)}); err != nil {
		t.Fatal(err)
	}

	sent := false
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		var frame map[string]json.RawMessage
		if err := conn.ReadJSON(&frame); err != nil {
			t.Fatalf("read: %v", err)
		}
		if typ, _ := frame["type"]; string(typ) == `"confirm_subscription"` && !sent {
			sent = true
			if _, err := client.SendMessage(ctx, uuid, "greet", 1000); err != nil {
				t.Fatal(err)
			}
			continue
		}
		msg, ok := frame["message"]
		if !ok {
			continue
		}
		var m struct {
			Type    string `json:"type"`
			Context *struct {
				Pct       float64 `json:"pct"`
				Count     int     `json:"count"`
				Threshold int     `json:"threshold"`
			} `json:"context"`
			Notifications *struct {
				Unread int `json:"unread"`
			} `json:"notifications"`
		}
		if json.Unmarshal(msg, &m) != nil || m.Type != "conversation.update" {
			continue
		}
		if m.Context == nil || m.Context.Threshold == 0 {
			t.Fatalf("conversation.update without usable context: %s", msg)
		}
		if m.Notifications == nil {
			t.Fatalf("conversation.update without notifications: %s", msg)
		}
		t.Logf("conversation.update: pct=%.1f count=%d unread=%d", m.Context.Pct, m.Context.Count, m.Notifications.Unread)
		return
	}
	t.Fatal("no conversation.update arrived within 45s")
}
