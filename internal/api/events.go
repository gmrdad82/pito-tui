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
	// Title and DisplayName mirror the Rails serializer (live-verified:
	// there is no "name" key). DisplayName is the human label.
	Title       string `json:"title"`
	DisplayName string `json:"display_name"`
}

// Label is the human-facing conversation name for status bars and pickers.
func (c Conversation) Label() string {
	if c.DisplayName != "" {
		return c.DisplayName
	}
	return c.Title
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
	DisplayName    string    `json:"display_name"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

// Label mirrors Conversation.Label for picker rows.
func (r ResumeRow) Label() string {
	if r.DisplayName != "" {
		return r.DisplayName
	}
	return r.Title
}

// ResumeList is GET /resume.json — Conversation.recency_groups serialized:
// everything within 24h of the most recent activity is "recent".
type ResumeList struct {
	Recent []ResumeRow `json:"recent"`
	Older  []ResumeRow `json:"older"`
}

// SendResult is the classified POST /chat reply. Live-verified: the server
// always answers {uuid, turn_id} 201, so "created" is a REQUEST-side fact —
// a blank-uuid send that came back with a uuid.
type SendResult struct {
	// TurnID identifies the in-flight turn (pending spinner bookkeeping).
	TurnID int64
	// CreatedUUID is set only when a blank-uuid send created the
	// conversation — the caller then fetches scrollback and subscribes.
	CreatedUUID string
	// Notice carries the server's error/message reply (web-only verbs and
	// friends) — rendered as a dim notice in the product's own voice, not
	// as an error.
	Notice *ServerNotice
}

// ServerNotice is the {error, message} reply shape (live-verified:
// {"error":"web_only","message":"That command wears a mouse cursor…"}).
type ServerNotice struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Verb    string `json:"verb"`
}

// Text is the line the UI shows — the server's prose when it sent some.
func (n ServerNotice) Text() string {
	if n.Message != "" {
		return n.Message
	}
	if n.Verb != "" {
		return n.Verb + " is web-only — open the web app for it"
	}
	return "the server declined that one (" + n.Error + ")"
}

// Suggestions is POST /suggestions — the server-driven palette. The
// grammar lives in verbs.yml server-side; the TUI never hardcodes verbs,
// it just renders what the ontology answers per keystroke.
type Suggestions struct {
	Mode      string       `json:"mode"`
	Stage     string       `json:"stage"`
	Ghost     Ghost        `json:"ghost"`
	MenuItems []Suggestion `json:"menu_items"`
}

type Ghost struct {
	CompleteCurrent string `json:"complete_current"`
	NextHint        string `json:"next_hint"`
}

type Suggestion struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Insert      string `json:"insert"`
	Masked      bool   `json:"masked"`
}
