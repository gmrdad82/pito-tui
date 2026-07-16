//go:build live

package app

// Live acceptance smoke against a real PITO instance — excluded from CI
// (build tag `live`), run by hand:
//
//	go test -tags live -run TestLiveSmoke -v ./internal/app/
//
// Environment: PITO_INSTANCE (default: a dev instance), PITO_OTP
// (default 123456 — the dev instance's fixed test code). Uses the real
// cookie jar at ~/.config/pito-tui/cookies.json, exactly like the binary.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
	"github.com/gmrdad82/pito-tui/internal/config"
)

func TestLiveSmoke(t *testing.T) {
	instance := os.Getenv("PITO_INSTANCE")
	if instance == "" {
		instance = "https://dev.pitomd.com"
	}
	otp := os.Getenv("PITO_OTP")
	if otp == "" {
		otp = "123456"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	client, err := api.New(instance, filepath.Join(dir, "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Session: reuse the jar; on 401 log in the way the TUI actually does —
	// an in-chat /login send through the server grammar (the reply's
	// Set-Cookie mints the session straight into the jar).
	if _, err := client.FetchResume(ctx, "", 0); errors.Is(err, api.ErrUnauthorized) {
		res, err := client.SendMessage(ctx, "", "/login "+otp, 80)
		if err != nil {
			t.Fatalf("in-chat /login failed: %v", err)
		}
		t.Logf("in-chat /login accepted (conversation %s)", res.CreatedUUID)
		if _, err := client.FetchResume(ctx, "", 0); err != nil {
			t.Fatalf("session did not stick after /login: %v", err)
		}
	} else if err != nil {
		t.Fatalf("resume: %v", err)
	}
	t.Log("session OK")

	// Blank-uuid send creates a conversation and processes the input.
	res, err := client.SendMessage(ctx, "", "hello from the pito-tui live smoke", 80)
	if err != nil {
		t.Fatalf("create-conversation send: %v", err)
	}
	if res.CreatedUUID == "" {
		t.Fatalf("expected a created uuid, got %+v", res)
	}
	uuid := res.CreatedUUID
	t.Logf("conversation created: %s", uuid)

	// The scrollback snapshot must contain the first turn's events.
	deadline := time.Now().Add(15 * time.Second)
	var page *api.ChatPage
	for {
		page, err = client.FetchChat(ctx, uuid)
		if err != nil {
			t.Fatalf("fetch scrollback: %v", err)
		}
		if len(page.Events) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Second)
	}
	if len(page.Events) == 0 {
		t.Fatal("scrollback stayed empty 15s after the first send")
	}
	t.Logf("scrollback: %d events, first kind %q", len(page.Events), page.Events[0].Kind)

	// Cable: subscribe, then send again and expect a live event.append.
	messages := make(chan cable.StreamMessage, 32)
	states := make(chan cable.ConnState, 8)
	cab, err := cable.New(cable.Config{
		InstanceURL: instance,
		UUID:        uuid,
		Jar:         client.Jar(),
		OnMessage:   func(m cable.StreamMessage) { messages <- m },
		OnState: func(s cable.ConnState, err error) {
			t.Logf("cable state: %v (err: %v)", s, err)
			states <- s
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = cab.Run(ctx) }()

	waitFor := func(want cable.ConnState, within time.Duration) {
		t.Helper()
		timeout := time.After(within)
		for {
			select {
			case s := <-states:
				if s == want {
					return
				}
			case <-timeout:
				t.Fatalf("cable never reached %v", want)
			}
		}
	}
	waitFor(cable.StateConnected, 15*time.Second)
	t.Log("cable subscription confirmed")

	if _, err := client.SendMessage(ctx, uuid, "ping over the cable", 80); err != nil {
		t.Fatalf("second send: %v", err)
	}
	timeout := time.After(20 * time.Second)
	for {
		select {
		case m := <-messages:
			t.Logf("cable delivered %s (event %d, kind %q)", m.Type, m.Event.ID, m.Event.Kind)
			if m.Type == cable.TypeEventAppend {
				return // full loop proven: send → server → stream → client
			}
		case <-timeout:
			t.Fatal("no event.append arrived over the cable within 20s")
		}
	}
}
