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
type SendResultMsg struct {
	Res *api.SendResult
	Err error
}

// ResumeFetchedMsg carries the conversation list for the picker.
type ResumeFetchedMsg struct {
	List *api.ResumeList
	Err  error
}
