package api

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "events", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestChatPageFixtureDecodes(t *testing.T) {
	var page ChatPage
	if err := json.Unmarshal(readFixture(t, "chat_page.json"), &page); err != nil {
		t.Fatal(err)
	}
	if page.Conversation.UUID != "3f1c9a2e-7b4d-4c1a-9e2f-8d5b6a7c9e01" {
		t.Errorf("uuid = %q", page.Conversation.UUID)
	}
	if page.Conversation.Name != "release prep" {
		t.Errorf("name = %q", page.Conversation.Name)
	}
	if len(page.Events) != 6 {
		t.Fatalf("events = %d, want 6", len(page.Events))
	}
	first := page.Events[0]
	if first.ID != 41 || first.TurnID != 7 || first.Kind != KindEcho {
		t.Errorf("first event = %+v", first)
	}
	if first.CreatedAt.IsZero() {
		t.Error("created_at did not parse")
	}
	// Events arrive in server (position) order; the client preserves it.
	for i := 1; i < len(page.Events); i++ {
		if page.Events[i].ID <= page.Events[i-1].ID {
			t.Errorf("event order broken at index %d", i)
		}
	}
}

// TestAllKindsDecode pins the contract invariant: every kind — including
// ones this client has never heard of — decodes without error, and the
// payload bytes survive round-tripping untouched.
func TestAllKindsDecode(t *testing.T) {
	raw := readFixture(t, "kinds.json")
	var events []Event
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 9 {
		t.Fatalf("events = %d, want 9", len(events))
	}

	known := map[string]bool{
		KindEcho: true, KindSystem: true, KindError: true, KindEnhanced: true,
		KindThinking: true, KindConfirmation: true, KindSystemFollowUp: true,
		KindEnhancedFollowUp: true, KindConfirmationFollowUp: true, KindThemeDiff: true,
	}
	sawUnknown := false
	for _, ev := range events {
		if !known[ev.Kind] {
			sawUnknown = true
			if ev.Kind != "holo_deck" {
				t.Errorf("unexpected unknown kind %q", ev.Kind)
			}
		}
		if len(ev.Payload) == 0 {
			t.Errorf("event %d: payload dropped", ev.ID)
		}
	}
	if !sawUnknown {
		t.Error("fixture must include an unknown kind to pin the fallback path")
	}

	// Raw payload bytes round-trip: what came in is what a renderer sees.
	var generic []struct {
		ID      int64           `json:"id"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	for i, ev := range events {
		var a, b bytes.Buffer
		if err := json.Compact(&a, ev.Payload); err != nil {
			t.Fatalf("event %d payload not valid JSON: %v", ev.ID, err)
		}
		if err := json.Compact(&b, generic[i].Payload); err != nil {
			t.Fatal(err)
		}
		if a.String() != b.String() {
			t.Errorf("event %d payload mutated:\n in: %s\nout: %s", ev.ID, b.String(), a.String())
		}
	}
}

func TestResumeFixtureDecodes(t *testing.T) {
	var list ResumeList
	if err := json.Unmarshal(readFixture(t, "resume.json"), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Recent) != 2 || len(list.Older) != 1 {
		t.Fatalf("recent/older = %d/%d, want 2/1", len(list.Recent), len(list.Older))
	}
	if list.Recent[0].Title != "release prep" {
		t.Errorf("title = %q", list.Recent[0].Title)
	}
	if list.Recent[0].LastActivityAt.IsZero() {
		t.Error("last_activity_at did not parse")
	}
}
