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
	if page.Conversation.Label() != "release prep" {
		t.Errorf("label = %q", page.Conversation.Label())
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

// TestAiBlockUnmarshalJSON pins AiBlock's custom decode in isolation: Raw
// always survives byte-for-byte, and Type is best-effort — a missing
// "type" key or a block that isn't even a JSON object never errors, it
// just leaves Type empty for the renderer to degrade.
func TestAiBlockUnmarshalJSON(t *testing.T) {
	cases := []struct {
		name     string
		json     string
		wantType string
	}{
		{"known type", `{"type":"score","value":82,"label":"sentiment"}`, "score"},
		{"unknown type", `{"type":"mystery_widget","glyph":"✦","weight":3.2}`, "mystery_widget"},
		{"missing type", `{"value":1}`, ""},
		{"not an object", `"just a string"`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b AiBlock
			if err := json.Unmarshal([]byte(c.json), &b); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if b.Type != c.wantType {
				t.Errorf("Type = %q, want %q", b.Type, c.wantType)
			}
			var want, got bytes.Buffer
			if err := json.Compact(&want, []byte(c.json)); err != nil {
				t.Fatal(err)
			}
			if err := json.Compact(&got, b.Raw); err != nil {
				t.Fatal(err)
			}
			if want.String() != got.String() {
				t.Errorf("Raw mutated:\n got: %s\nwant: %s", got.String(), want.String())
			}
		})
	}
}

// TestDecodeAiPayload is the table-driven contract test for the "ai" kind
// (pito 2.0.0, kind "ai"). Each case decodes a payload and asserts on the
// result — see w2-contract.md for the wire shape this pins.
func TestDecodeAiPayload(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		check func(t *testing.T, p AiPayload)
	}{
		{
			name: "full done payload",
			body: `{
				"status": "done",
				"prompt": "how's Hades II doing this week?",
				"model": "claude-sonnet-5",
				"provider": "anthropic",
				"effort": "medium",
				"cost_amount": 0.0421,
				"cost_currency": "USD",
				"reply_handle": "@ai-88",
				"reply_consumed": false,
				"anchor_event_id": 41,
				"blocks": [
					{"type": "text", "text": "**Hades II** is [subject]trending up[/subject] this week."},
					{"type": "kv_table", "rows": [["views", {"v": 128000, "format": "number"}], ["title", "Hades II"]]},
					{"type": "table", "header": ["video", "views"], "rows": [["ep 12", "8.1k"], ["ep 13", "9.4k"]]},
					{"type": "media", "entity": "vid", "id": 12, "variant": "thumb"},
					{"type": "sparkline", "series": [1.0, 2.5, 3.2, 2.8], "label": "views/day", "series_max": 4.0},
					{"type": "chart", "viz": "area", "series": [1, 2, 3], "dates": ["2026-07-01", "2026-07-02", "2026-07-03"], "label": "views"},
					{"type": "score", "value": 82, "label": "sentiment"},
					{"type": "ttb", "levels": [{"label": "bronze", "hours": 10}, {"label": "silver", "hours": 40}], "current": {"label": "bronze", "hours": 12}, "label": "time to next badge"},
					{"type": "suggestion", "command": "/analyze hades-2", "note": "deep dive"}
				]
			}`,
			check: func(t *testing.T, p AiPayload) {
				if p.Status != "done" {
					t.Errorf("status = %q, want done", p.Status)
				}
				if p.Prompt != "how's Hades II doing this week?" {
					t.Errorf("prompt = %q", p.Prompt)
				}
				if p.Model != "claude-sonnet-5" || p.Provider != "anthropic" || p.Effort != "medium" {
					t.Errorf("model/provider/effort = %q/%q/%q", p.Model, p.Provider, p.Effort)
				}
				if p.CostAmount == nil || *p.CostAmount != 0.0421 {
					t.Errorf("cost_amount = %v, want 0.0421", p.CostAmount)
				}
				if p.CostCurrency != "USD" {
					t.Errorf("cost_currency = %q", p.CostCurrency)
				}
				if p.ReplyHandle != "@ai-88" || p.ReplyConsumed {
					t.Errorf("reply_handle/reply_consumed = %q/%v", p.ReplyHandle, p.ReplyConsumed)
				}
				if p.AnchorEventID != 41 {
					t.Errorf("anchor_event_id = %d, want 41", p.AnchorEventID)
				}
				wantTypes := []string{
					"text", "kv_table", "table", "media", "sparkline",
					"chart", "score", "ttb", "suggestion",
				}
				if len(p.Blocks) != len(wantTypes) {
					t.Fatalf("blocks = %d, want %d", len(p.Blocks), len(wantTypes))
				}
				for i, want := range wantTypes {
					if p.Blocks[i].Type != want {
						t.Errorf("block %d type = %q, want %q", i, p.Blocks[i].Type, want)
					}
					if len(p.Blocks[i].Raw) == 0 {
						t.Errorf("block %d: raw dropped", i)
					}
				}
			},
		},
		{
			name: "pending payload",
			body: `{
				"status": "pending",
				"prompt": "how's the channel doing",
				"model": "claude-sonnet-5",
				"provider": "anthropic",
				"blocks": [
					{"type": "text", "text": "Working on it…"}
				]
			}`,
			check: func(t *testing.T, p AiPayload) {
				if p.Status != "pending" {
					t.Errorf("status = %q, want pending", p.Status)
				}
				if len(p.Blocks) != 1 || p.Blocks[0].Type != "text" {
					t.Fatalf("blocks = %+v", p.Blocks)
				}
				if p.CostAmount != nil {
					t.Errorf("cost_amount = %v, want nil (not settled yet)", *p.CostAmount)
				}
				if p.AnchorEventID != 0 || p.ReplyHandle != "" {
					t.Errorf("anchor_event_id/reply_handle should be zero-valued, got %d/%q", p.AnchorEventID, p.ReplyHandle)
				}
			},
		},
		{
			name: "unknown block type preserved raw",
			body: `{
				"status": "done",
				"blocks": [
					{"type": "mystery_widget", "glyph": "✦", "weight": 3.2, "nested": {"a": 1}}
				]
			}`,
			check: func(t *testing.T, p AiPayload) {
				if len(p.Blocks) != 1 {
					t.Fatalf("blocks = %d, want 1", len(p.Blocks))
				}
				b := p.Blocks[0]
				if b.Type != "mystery_widget" {
					t.Errorf("type = %q, want mystery_widget", b.Type)
				}
				var want, got bytes.Buffer
				if err := json.Compact(&want, []byte(`{"type": "mystery_widget", "glyph": "✦", "weight": 3.2, "nested": {"a": 1}}`)); err != nil {
					t.Fatal(err)
				}
				if err := json.Compact(&got, b.Raw); err != nil {
					t.Fatal(err)
				}
				if want.String() != got.String() {
					t.Errorf("raw mutated:\n got: %s\nwant: %s", got.String(), want.String())
				}
			},
		},
		{
			name: "malformed block does not error the payload",
			body: `{
				"status": "done",
				"blocks": [
					{"type": "text", "text": "before"},
					"just a string, not a block object",
					{"type": "text", "text": "after"}
				]
			}`,
			check: func(t *testing.T, p AiPayload) {
				if len(p.Blocks) != 3 {
					t.Fatalf("blocks = %d, want 3", len(p.Blocks))
				}
				if p.Blocks[1].Type != "" {
					t.Errorf("malformed block type = %q, want empty", p.Blocks[1].Type)
				}
				if string(p.Blocks[1].Raw) != `"just a string, not a block object"` {
					t.Errorf("malformed block raw = %s", p.Blocks[1].Raw)
				}
				if p.Blocks[0].Type != "text" || p.Blocks[2].Type != "text" {
					t.Errorf("neighboring blocks corrupted: %+v", p.Blocks)
				}
			},
		},
		{
			name: "typed kv values pass through raw",
			body: `{
				"status": "done",
				"blocks": [
					{"type": "kv_table", "rows": [
						["title", "Hades II"],
						["revenue", {"v": 42.1, "format": "price"}],
						["updated", {"v": "2026-07-10", "format": "date"}]
					]}
				]
			}`,
			check: func(t *testing.T, p AiPayload) {
				if len(p.Blocks) != 1 || p.Blocks[0].Type != "kv_table" {
					t.Fatalf("blocks = %+v", p.Blocks)
				}
				var kv struct {
					Rows [][2]json.RawMessage `json:"rows"`
				}
				if err := json.Unmarshal(p.Blocks[0].Raw, &kv); err != nil {
					t.Fatalf("kv_table raw did not decode: %v", err)
				}
				if len(kv.Rows) != 3 {
					t.Fatalf("rows = %d, want 3", len(kv.Rows))
				}
				// A plain string value survives as a raw JSON string...
				if string(kv.Rows[0][1]) != `"Hades II"` {
					t.Errorf("row 0 value = %s, want a raw JSON string", kv.Rows[0][1])
				}
				// ...and a typed {v, format} hash survives as a raw JSON
				// object — the api package never collapses it to a string.
				var priceCheck, dateCheck bytes.Buffer
				if err := json.Compact(&priceCheck, kv.Rows[1][1]); err != nil {
					t.Fatal(err)
				}
				if priceCheck.String() != `{"v":42.1,"format":"price"}` {
					t.Errorf("row 1 value = %s", priceCheck.String())
				}
				if err := json.Compact(&dateCheck, kv.Rows[2][1]); err != nil {
					t.Fatal(err)
				}
				if dateCheck.String() != `{"v":"2026-07-10","format":"date"}` {
					t.Errorf("row 2 value = %s", dateCheck.String())
				}
			},
		},
		{
			name: "tolerant decode of unknown top-level fields",
			body: `{
				"status": "done",
				"prompt": "hi",
				"future_field": "the server shipped this before we knew about it",
				"another_new_key": {"nested": true, "n": [1, 2, 3]},
				"blocks": []
			}`,
			check: func(t *testing.T, p AiPayload) {
				if p.Status != "done" || p.Prompt != "hi" {
					t.Errorf("known fields did not survive unknown siblings: status=%q prompt=%q", p.Status, p.Prompt)
				}
				if len(p.Blocks) != 0 {
					t.Errorf("blocks = %d, want 0", len(p.Blocks))
				}
			},
		},
		{
			name: "cost fields absent",
			body: `{"status": "pending", "blocks": []}`,
			check: func(t *testing.T, p AiPayload) {
				if p.CostAmount != nil {
					t.Errorf("cost_amount = %v, want nil", *p.CostAmount)
				}
				if p.CostCurrency != "" {
					t.Errorf("cost_currency = %q, want empty", p.CostCurrency)
				}
			},
		},
		{
			name: "cost fields present",
			body: `{"status": "done", "blocks": [], "cost_amount": 3.5, "cost_currency": "USD"}`,
			check: func(t *testing.T, p AiPayload) {
				if p.CostAmount == nil || *p.CostAmount != 3.5 {
					t.Errorf("cost_amount = %v, want 3.5", p.CostAmount)
				}
				if p.CostCurrency != "USD" {
					t.Errorf("cost_currency = %q, want USD", p.CostCurrency)
				}
			},
		},
		{
			name: "cost fields zero",
			body: `{"status": "done", "blocks": [], "cost_amount": 0, "cost_currency": "USD"}`,
			check: func(t *testing.T, p AiPayload) {
				// A free model still settles at $0.00 — that must decode to
				// a non-nil pointer to 0, distinguishable from "absent".
				if p.CostAmount == nil {
					t.Fatal("cost_amount = nil, want non-nil pointer to 0")
				}
				if *p.CostAmount != 0 {
					t.Errorf("cost_amount = %v, want 0", *p.CostAmount)
				}
			},
		},
		{
			name: "cost_estimated absent defaults false",
			body: `{"status": "done", "blocks": [], "cost_amount": 0.03, "cost_currency": "USD"}`,
			check: func(t *testing.T, p AiPayload) {
				// A reported provider receipt carries no flag at all — must
				// decode to false, not error or leave it ambiguous.
				if p.CostEstimated {
					t.Errorf("cost_estimated = %v, want false (absent key)", p.CostEstimated)
				}
			},
		},
		{
			name: "cost_estimated true",
			body: `{"status": "done", "blocks": [], "cost_amount": 0.03, "cost_currency": "USD", "cost_estimated": true}`,
			check: func(t *testing.T, p AiPayload) {
				if !p.CostEstimated {
					t.Errorf("cost_estimated = %v, want true", p.CostEstimated)
				}
			},
		},
		{
			name: "cost_estimated false",
			body: `{"status": "done", "blocks": [], "cost_amount": 0.03, "cost_currency": "USD", "cost_estimated": false}`,
			check: func(t *testing.T, p AiPayload) {
				if p.CostEstimated {
					t.Errorf("cost_estimated = %v, want false", p.CostEstimated)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := DecodeAiPayload(json.RawMessage(c.body))
			if err != nil {
				t.Fatalf("DecodeAiPayload: %v", err)
			}
			c.check(t, p)
		})
	}
}

// TestDecodeSystemPayload is the table-driven contract test for kind
// "system"'s hits card (search_conversations' reply, pito-tui 3.0.1 fix —
// see hitpicker.go). The fixtures below are the REAL contract:
// table_heading/table_rows, mirroring
// Pito::MessageBuilder::Conversation::Hits (pito
// lib/pito/message_builder/conversation/hits.rb) exactly as its own spec
// (spec/lib/pito/message_builder/conversation/hits_spec.rb) pins the
// shape — a like-mode row's cells `[{text, class, data}, {score}]` plus
// row-level `data: {anchor_event_id, conversation_uuid}`; a for-mode row's
// second cell `{text, class}` (an occurrence count, no score). Mirrors
// TestDecodeAiPayload's table shape: hits present decode to the full
// slice; hits absent — an old server, or any system event that isn't a
// hits card — decode to an empty slice rather than erroring
// (hitsFromLatestEvent, hitpicker.go, only opens the picker when this
// comes back non-empty); and nothing here ever panics, even a payload
// shaped so wrong it has to surface as a decode error.
func TestDecodeSystemPayload(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		check func(t *testing.T, p SystemPayload, err error)
	}{
		{
			name: "like-mode hits present (score cells)",
			body: `{
				"body": "2 conversations found.",
				"html": true,
				"table_heading": ["Conversation", "Score"],
				"table_rows": [
					{
						"cells": [
							{
								"text": "Hades II thoughts",
								"class": "pito-action-shimmer pito-shimmer-d5 pito-cell-title",
								"data": {
									"controller": "pito--chat-prefill",
									"action": "click->pito--chat-prefill#fill",
									"pito--chat-prefill-text-value": "/resume conv-uuid-1",
									"pito--chat-prefill-submit-value": "true"
								}
							},
							{"score": 87}
						],
						"data": {"anchor_event_id": 501, "conversation_uuid": "conv-uuid-1"}
					},
					{
						"cells": [
							{
								"text": "Hades II vs Hades I",
								"class": "pito-action-shimmer pito-shimmer-d2 pito-cell-title",
								"data": {
									"controller": "pito--chat-prefill",
									"action": "click->pito--chat-prefill#fill",
									"pito--chat-prefill-text-value": "/resume conv-uuid-2",
									"pito--chat-prefill-submit-value": "true"
								}
							},
							{"score": 42}
						],
						"data": {"anchor_event_id": 999, "conversation_uuid": "conv-uuid-2"}
					}
				],
				"list_footer": "Reply with a hit's row number."
			}`,
			check: func(t *testing.T, p SystemPayload, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(p.Hits) != 2 {
					t.Fatalf("hits = %d, want 2", len(p.Hits))
				}
				first := p.Hits[0]
				if first.ConversationUUID != "conv-uuid-1" || first.AnchorEventID != 501 || first.Title != "Hades II thoughts" {
					t.Errorf("hit 0 = %+v", first)
				}
				if first.Score == nil || *first.Score != 87 {
					t.Errorf("hit 0 score = %v, want 87", first.Score)
				}
				if first.OccurrenceCount != nil {
					t.Errorf("hit 0 occurrence_count = %v, want nil (like mode)", *first.OccurrenceCount)
				}
				second := p.Hits[1]
				if second.ConversationUUID != "conv-uuid-2" || second.AnchorEventID != 999 || second.Title != "Hades II vs Hades I" {
					t.Errorf("hit 1 = %+v", second)
				}
				if second.Score == nil || *second.Score != 42 {
					t.Errorf("hit 1 score = %v, want 42", second.Score)
				}
			},
		},
		{
			name: "for-mode hits present (occurrence-count cells)",
			body: `{
				"body": "1 conversation found.",
				"html": true,
				"table_heading": ["Conversation", "Occurrences"],
				"table_rows": [
					{
						"cells": [
							{
								"text": "Hades II thoughts",
								"class": "pito-action-shimmer pito-shimmer-d5 pito-cell-title",
								"data": {
									"controller": "pito--chat-prefill",
									"action": "click->pito--chat-prefill#fill",
									"pito--chat-prefill-text-value": "/resume conv-uuid-1 501",
									"pito--chat-prefill-submit-value": "true"
								}
							},
							{"text": "3", "class": "tabular-nums text-right whitespace-nowrap"}
						],
						"data": {"anchor_event_id": 501, "conversation_uuid": "conv-uuid-1"}
					}
				],
				"list_footer": "Reply with a hit's row number."
			}`,
			check: func(t *testing.T, p SystemPayload, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(p.Hits) != 1 {
					t.Fatalf("hits = %d, want 1", len(p.Hits))
				}
				hit := p.Hits[0]
				if hit.ConversationUUID != "conv-uuid-1" || hit.AnchorEventID != 501 || hit.Title != "Hades II thoughts" {
					t.Errorf("hit = %+v", hit)
				}
				if hit.OccurrenceCount == nil || *hit.OccurrenceCount != 3 {
					t.Errorf("occurrence_count = %v, want 3", hit.OccurrenceCount)
				}
				if hit.Score != nil {
					t.Errorf("score = %v, want nil (for mode)", *hit.Score)
				}
			},
		},
		{
			name: "hits absent — an old server or a non-hits system event",
			body: `{"text": "pong"}`,
			check: func(t *testing.T, p SystemPayload, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(p.Hits) != 0 {
					t.Errorf("hits = %+v, want empty", p.Hits)
				}
			},
		},
		{
			name: "hits explicitly empty",
			body: `{"body": "no matches", "html": true, "table_heading": ["Conversation", "Score"], "table_rows": []}`,
			check: func(t *testing.T, p SystemPayload, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(p.Hits) != 0 {
					t.Errorf("hits = %+v, want empty", p.Hits)
				}
			},
		},
		{
			// A plain list card (e.g. `ls games`) is ALSO kind "system" with
			// table_rows, and its cells carry the SAME cell-level `data`
			// prefill token a hits row's name cell does — but hits.rb is
			// the only builder that ALSO stamps row-level `data`
			// (anchor_event_id + conversation_uuid). Without that row-level
			// contract this must NOT be mistaken for a hits card — the
			// exact bug class this rewrite exists to prevent (a client that
			// trusted the row shape alone, not the row-level data, could
			// misfire the picker on any list reply).
			name: "a plain list card's table_rows (no row-level data) never counts as hits",
			body: `{
				"body": "8 vids.",
				"html": true,
				"table_heading": ["#", "Title"],
				"table_rows": [
					{
						"cells": [
							{
								"text": "#18",
								"class": "pito-action-shimmer pito-shimmer-d7 tabular-nums text-right whitespace-nowrap",
								"data": {
									"controller": "pito--chat-prefill",
									"action": "click->pito--chat-prefill#fill",
									"pito--chat-prefill-text-value": "show vid #18",
									"pito--chat-prefill-submit-value": "true"
								}
							},
							{"text": "Ghosts 'n Goblins Resurrection : Knight", "class": "text-fg pito-cell-title"}
						]
					}
				]
			}`,
			check: func(t *testing.T, p SystemPayload, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(p.Hits) != 0 {
					t.Errorf("hits = %+v, want empty (this is a list card, not a hits card)", p.Hits)
				}
			},
		},
		{
			name: "tolerant of unknown top-level fields",
			body: `{
				"table_heading": ["Conversation", "Score"],
				"table_rows": [
					{
						"cells": [{"text": "t"}, {"score": 10}],
						"data": {"anchor_event_id": 501, "conversation_uuid": "u"}
					}
				],
				"future_field": true
			}`,
			check: func(t *testing.T, p SystemPayload, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(p.Hits) != 1 {
					t.Fatalf("hits = %d, want 1", len(p.Hits))
				}
			},
		},
		{
			// hitRowWire carries no custom UnmarshalJSON (unlike AiBlock),
			// so a table_rows entry shaped wrong for the row's own fields
			// does surface as a decode error here — the point of this case
			// is only that it does so cleanly (an error return), never a
			// panic. The caller already treats any error as "no hits"
			// (hitsFromLatestEvent), so a malformed row degrades to the J
			// key falling through, same as an absent list.
			name: "malformed table_rows entries error without panicking",
			body: `{
				"table_rows": [
					{"cells": [{"text": "ok row"}, {"score": 1}], "data": {"anchor_event_id": 501, "conversation_uuid": "u"}},
					"just a string, not a row object"
				]
			}`,
			check: func(t *testing.T, p SystemPayload, err error) {
				if err == nil {
					t.Fatal("want a decode error for a malformed table_rows array, got nil")
				}
			},
		},
		{
			name: "payload not an object at all",
			body: `"just a string"`,
			check: func(t *testing.T, p SystemPayload, err error) {
				if err == nil {
					t.Fatal("want a decode error for a non-object payload, got nil")
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := DecodeSystemPayload(json.RawMessage(c.body))
			c.check(t, p, err)
		})
	}
}

// TestKindAiDecodesAsEvent pins kind "ai" flowing through the generic Event
// envelope like any other kind — DecodeAiPayload then takes over on the
// raw payload bytes.
func TestKindAiDecodesAsEvent(t *testing.T) {
	raw := []byte(`{"id": 99, "turn_id": 10, "kind": "ai",
		"payload": {"status": "done", "blocks": [{"type": "text", "text": "hi"}]},
		"created_at": "2026-07-10T10:00:00.000Z"}`)
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindAi {
		t.Errorf("kind = %q, want %q", ev.Kind, KindAi)
	}
	p, err := DecodeAiPayload(ev.Payload)
	if err != nil {
		t.Fatalf("DecodeAiPayload: %v", err)
	}
	if p.Status != "done" || len(p.Blocks) != 1 {
		t.Errorf("payload = %+v", p)
	}
}

// TestResumeRowAIFlag pins ResumeRow.AI's tolerant decode (tui-needs ask
// 9b): the wire key isn't named yet, so both "ai" and "has_ai" must decode,
// and anything else — absence, false, or a non-bool value — degrades to
// false without erroring the row.
func TestResumeRowAIFlag(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"absent", `{"uuid": "a"}`, false},
		{`"ai": true`, `{"uuid": "a", "ai": true}`, true},
		{`"has_ai": true`, `{"uuid": "a", "has_ai": true}`, true},
		{"both false", `{"uuid": "a", "ai": false, "has_ai": false}`, false},
		{"non-bool garbage", `{"uuid": "a", "ai": "yes"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var row ResumeRow
			if err := json.Unmarshal([]byte(c.body), &row); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if row.AI != c.want {
				t.Errorf("AI = %v, want %v", row.AI, c.want)
			}
			if row.UUID != "a" {
				t.Errorf("uuid = %q, want unaffected sibling field to survive", row.UUID)
			}
		})
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

// TestSuggestionModelFieldDecodes pins the model-mention wire contract's
// client half: the @ai menu item gains an ADDITIVE optional "model"
// field, absent on every other item and absent when no model is
// configured — old servers that never send it, and menu items that never
// carry it, must decode to "" without error either way.
func TestSuggestionModelFieldDecodes(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantModel string
	}{
		{
			name:      "ai item with a configured model",
			body:      `{"label":"@ai","description":"ask claude-sonnet-5 anything","insert":"@ai ","model":"claude-sonnet-5"}`,
			wantModel: "claude-sonnet-5",
		},
		{
			name:      "ai item, no model configured — field absent",
			body:      `{"label":"@ai","description":"ask the assistant anything","insert":"@ai "}`,
			wantModel: "",
		},
		{
			name:      "non-ai item — field never present, old server shape",
			body:      `{"label":"/config","description":"open settings","insert":"/config"}`,
			wantModel: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var s Suggestion
			if err := json.Unmarshal([]byte(c.body), &s); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.Model != c.wantModel {
				t.Errorf("Model = %q, want %q", s.Model, c.wantModel)
			}
		})
	}
}
