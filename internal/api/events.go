package api

import (
	"encoding/json"
	"time"
)

// Known event kinds (Event::KINDS on the Rails side). The TUI renders these
// specially and falls back to a generic renderer for anything else — an
// unknown kind must degrade, never crash, so old clients survive new
// servers.
const (
	KindEcho                 = "echo"
	KindSystem               = "system"
	KindError                = "error"
	KindEnhanced             = "enhanced"
	KindThinking             = "thinking"
	KindConfirmation         = "confirmation"
	KindSystemFollowUp       = "system_follow_up"
	KindEnhancedFollowUp     = "enhanced_follow_up"
	KindConfirmationFollowUp = "confirmation_follow_up"
	KindThemeDiff            = "theme_diff"
)

// Event is the canonical scrollback unit. Payload stays raw: kind-specific
// decoding happens in the renderers, so decoding a page can never fail on
// an unknown kind.
type Event struct {
	ID        int64           `json:"id"`
	TurnID    int64           `json:"turn_id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type Conversation struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// ChatPage is GET /chat/:uuid.json — the scrollback snapshot used for the
// initial paint and for every reconnect re-sync (the cable has no replay).
type ChatPage struct {
	Conversation Conversation `json:"conversation"`
	Events       []Event      `json:"events"`
}

// ResumeRow is one conversation in the picker.
type ResumeRow struct {
	UUID           string    `json:"uuid"`
	Title          string    `json:"title"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

// ResumeList is GET /resume.json — Conversation.recency_groups serialized:
// everything within 24h of the most recent activity is "recent".
type ResumeList struct {
	Recent []ResumeRow `json:"recent"`
	Older  []ResumeRow `json:"older"`
}

// SendResult is the classified POST /chat reply.
type SendResult struct {
	// TurnID is set on the normal {accepted, turn_id} ack.
	TurnID int64
	// CreatedUUID is set when a blank-uuid send created the conversation
	// ({uuid} 201) — the caller then fetches scrollback and subscribes.
	CreatedUUID string
	// WebOnly carries the server's web-only-verb notice ({error:
	// "web-only", verb}) — rendered as a dim notice, not an error.
	WebOnly *WebOnlyNotice
}

type WebOnlyNotice struct {
	Error string `json:"error"`
	Verb  string `json:"verb"`
}
