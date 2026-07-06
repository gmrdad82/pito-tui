// Package cable is a minimal ActionCable client — just enough of the wire
// protocol for one subscription: welcome, subscribe, confirm, ping
// tracking, and message dispatch. No external cable libraries; the
// protocol is small and owning it beats depending on an abandoned wrapper.
//
// The package never imports Bubble Tea: it emits through callbacks and the
// app layer forwards them with tea.Program.Send. That keeps protocol tests
// pure and lets UI tests inject cable traffic directly.
package cable

import (
	"encoding/json"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// ChannelName is THE single constant mirroring the Rails channel class
// (Pito::JsonChannel — app/channels/pito/json_channel.rb, live-verified).
// It subscribes by bare uuid and rejects unauthenticated connections.
// Change both sides or neither.
const ChannelName = "Pito::JsonChannel"

// Subprotocol is ActionCable's JSON wire protocol identifier.
const Subprotocol = "actioncable-v1-json"

// Stream message types broadcast on "pito:json:conversation:<uuid>".
const (
	TypeEventAppend  = "event.append"
	TypeEventReplace = "event.replace"
)

// StreamMessage is one broadcast from the conversation's JSON stream.
// Unknown Type values pass through — the UI ignores what it doesn't know.
type StreamMessage struct {
	Type  string    `json:"type"`
	Event api.Event `json:"event"`
	// conversation.update fields (context meter + notifications; the
	// message carries no event). Nil on event.* messages.
	Context       *api.ContextMeter `json:"context"`
	Notifications *api.NotifCount   `json:"notifications"`
}

type ConnState int

const (
	StateConnecting ConnState = iota
	// StateConnected means the subscription is CONFIRMED — a TCP-level
	// connect that never confirms stays "connecting" for the UI.
	StateConnected
	StateDisconnected
)

func (s ConnState) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	default:
		return "disconnected"
	}
}

// Identifier renders the subscribe identifier for a conversation. Action
// Cable quirk: the identifier is a JSON-encoded STRING inside the frame,
// not a nested object.
func Identifier(uuid string) string {
	raw, _ := json.Marshal(struct {
		Channel string `json:"channel"`
		UUID    string `json:"uuid"`
	}{ChannelName, uuid})
	return string(raw)
}

// frame is the inbound envelope. Message must stay raw: pings put a bare
// number there while broadcasts put an object — a typed field would crash
// on one or the other.
type frame struct {
	Type       string          `json:"type"`
	Identifier string          `json:"identifier"`
	Message    json.RawMessage `json:"message"`
	Reason     string          `json:"reason"`
}

// TypeConversationUpdate is the per-turn meter/notification patch
// (tui-needs.md item 1) — no event rides on it.
const TypeConversationUpdate = "conversation.update"
