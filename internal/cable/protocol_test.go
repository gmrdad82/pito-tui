package cable

import (
	"encoding/json"
	"testing"
)

func TestIdentifierExactShape(t *testing.T) {
	got := Identifier("3f1c")
	want := `{"channel":"Pito::JsonChannel","uuid":"3f1c"}`
	if got != want {
		t.Errorf("Identifier = %s, want %s", got, want)
	}
}

func TestConnStateString(t *testing.T) {
	cases := map[ConnState]string{
		StateConnecting:   "connecting",
		StateConnected:    "connected",
		StateDisconnected: "disconnected",
		ConnState(99):     "disconnected",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", state, got, want)
		}
	}
}

func TestDecodeEventAiBlock(t *testing.T) {
	const block = `{"kind":"tool_use","name":"web_search","status":"running"}`
	raw := `{"type":"event.ai_block","event_id":123,"index":3,"block":` + block + `}`

	var sm StreamMessage
	if err := json.Unmarshal([]byte(raw), &sm); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if sm.Type != TypeEventAiBlock {
		t.Errorf("Type = %q, want %q", sm.Type, TypeEventAiBlock)
	}
	if sm.EventID != 123 {
		t.Errorf("EventID = %d, want 123", sm.EventID)
	}
	if sm.Index != 3 {
		t.Errorf("Index = %d, want 3", sm.Index)
	}
	if string(sm.Block) != block {
		t.Errorf("Block = %s, want byte-for-byte %s", sm.Block, block)
	}
	if sm.Text != "" {
		t.Errorf("Text = %q, want empty", sm.Text)
	}
}

func TestDecodeEventAiBlockIndexDefaultsZero(t *testing.T) {
	raw := `{"type":"event.ai_block","event_id":7,"block":{"kind":"text"}}`

	var sm StreamMessage
	if err := json.Unmarshal([]byte(raw), &sm); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if sm.Index != 0 {
		t.Errorf("Index = %d, want 0 (omitted field defaults)", sm.Index)
	}
}

func TestDecodeEventAiStatus(t *testing.T) {
	raw := `{"type":"event.ai_status","event_id":123,"text":"Scouring the internet…"}`

	var sm StreamMessage
	if err := json.Unmarshal([]byte(raw), &sm); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if sm.Type != TypeEventAiStatus {
		t.Errorf("Type = %q, want %q", sm.Type, TypeEventAiStatus)
	}
	if sm.EventID != 123 {
		t.Errorf("EventID = %d, want 123", sm.EventID)
	}
	if sm.Text != "Scouring the internet…" {
		t.Errorf("Text = %q, want %q", sm.Text, "Scouring the internet…")
	}
	if sm.Block != nil {
		t.Errorf("Block = %s, want nil on a status message", sm.Block)
	}
}

// TestDecodeEventAiBlockMalformedBlockDoesNotError guards the "capability-
// tolerant" contract: block is opaque to this layer (the UI decodes it), so
// a shape this side doesn't expect — a bare string here, a future server
// might send anything — must still decode without erroring the message.
func TestDecodeEventAiBlockMalformedBlockDoesNotError(t *testing.T) {
	raw := `{"type":"event.ai_block","event_id":1,"index":0,"block":"not-an-object"}`

	var sm StreamMessage
	if err := json.Unmarshal([]byte(raw), &sm); err != nil {
		t.Fatalf("Unmarshal: %v, want no error on an unexpected block shape", err)
	}
	if string(sm.Block) != `"not-an-object"` {
		t.Errorf("Block = %s, want raw passthrough of the string", sm.Block)
	}
}

// TestDecodeExistingTypesUnchanged is a regression guard: adding the
// event_id/index/block/text fields to StreamMessage must not disturb
// event.append/event.replace/conversation.update decoding.
func TestDecodeExistingTypesUnchanged(t *testing.T) {
	raw := `{"type":"event.append","event":{"id":42,"kind":"system"}}`

	var sm StreamMessage
	if err := json.Unmarshal([]byte(raw), &sm); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if sm.Type != TypeEventAppend || sm.Event.ID != 42 || sm.Event.Kind != "system" {
		t.Errorf("event.append decode = %+v", sm)
	}
	if sm.EventID != 0 || sm.Index != 0 || sm.Block != nil || sm.Text != "" {
		t.Errorf("event.append leaked ai fields: %+v", sm)
	}
}
