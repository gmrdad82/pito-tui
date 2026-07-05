package ui

import (
	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
)

// CableEventMsg wraps one append/replace broadcast. The app layer forwards
// cable callbacks here via tea.Program.Send.
type CableEventMsg struct {
	M cable.StreamMessage
}

// ConnStateMsg reports cable lifecycle transitions.
type ConnStateMsg struct {
	State cable.ConnState
	Err   error
}

// ChatFetchedMsg carries a scrollback snapshot; Resync marks a
// reconnect-triggered refetch that must merge instead of paint.
type ChatFetchedMsg struct {
	Page   *api.ChatPage
	Resync bool
	Err    error
}

// SendResultMsg is POST /chat's classified reply (or its failure).
// Input carries what was typed — mutation replies (turn_id null) create
// no server echo, so the client echoes them itself.
type SendResultMsg struct {
	Res   *api.SendResult
	Err   error
	Input string
}

// ResumeFetchedMsg carries the conversation list for the picker.
type ResumeFetchedMsg struct {
	List *api.ResumeList
	Err  error
}

// AnimTickMsg drives the shimmer sweep — emitted only while fresh
// shimmer-marked turns are on screen (then the loop dies).
type AnimTickMsg struct{}

// SuggestionsMsg carries a palette reply; Seq guards against stale
// responses overtaking fresh keystrokes.
type SuggestionsMsg struct {
	Seq int
	S   *api.Suggestions
}
