package ui

import (
	"bytes"
	"strings"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// RenderFunc turns one event into its terminal block at the given width.
type RenderFunc func(ev api.Event, width int) string

// Transcript is the pure scrollback store: turns in server order, events
// within their turn, and a per-turn render cache so an append or replace
// re-renders exactly one turn. It has no Bubble Tea dependencies — unit
// tests drive it directly.
type Transcript struct {
	turns   []*Turn
	byTurn  map[int64]*Turn
	byEvent map[int64]eventPos
	render  RenderFunc
	width   int

	joined   string
	joinedOK bool
}

type Turn struct {
	ID     int64
	Events []api.Event

	rendered string
	dirty    bool
}

type eventPos struct {
	turn *Turn
	idx  int
}

func NewTranscript(render RenderFunc) *Transcript {
	return &Transcript{
		byTurn:  map[int64]*Turn{},
		byEvent: map[int64]eventPos{},
		render:  render,
	}
}

// SetRenderer swaps the renderer (terminal resize) and invalidates every
// cached block.
func (t *Transcript) SetRenderer(render RenderFunc) {
	t.render = render
	t.dirtyAll()
}

// Append adds an event to its turn, creating the turn block on its first
// event (mirrors the web's turn containers). Duplicate IDs are dropped —
// that idempotency is what makes the reconnect re-sync race-free.
func (t *Transcript) Append(ev api.Event) {
	if _, dup := t.byEvent[ev.ID]; dup {
		return
	}
	turn := t.byTurn[ev.TurnID]
	if turn == nil {
		turn = &Turn{ID: ev.TurnID}
		t.byTurn[ev.TurnID] = turn
		t.turns = append(t.turns, turn)
	}
	turn.Events = append(turn.Events, ev)
	t.byEvent[ev.ID] = eventPos{turn: turn, idx: len(turn.Events) - 1}
	turn.dirty = true
	t.joinedOK = false
}

// Replace rewrites an event in place (event.replace — confirmations
// flipping to processing/resolved). An unseen ID appends defensively.
func (t *Transcript) Replace(ev api.Event) {
	pos, ok := t.byEvent[ev.ID]
	if !ok {
		t.Append(ev)
		return
	}
	pos.turn.Events[pos.idx] = ev
	pos.turn.dirty = true
	t.joinedOK = false
}

// Merge applies a freshly-fetched scrollback page over the live transcript
// (reconnect re-sync — the cable has no replay, HTTP is the source of
// truth). Unknown IDs append, known IDs with changed payloads replace,
// identical events no-op. Returns how many events changed.
func (t *Transcript) Merge(events []api.Event) int {
	changed := 0
	for _, ev := range events {
		pos, known := t.byEvent[ev.ID]
		switch {
		case !known:
			t.Append(ev)
			changed++
		case !bytes.Equal(pos.turn.Events[pos.idx].Payload, ev.Payload):
			t.Replace(ev)
			changed++
		}
	}
	return changed
}

// HasTurn reports whether any event of the turn has arrived (pending
// spinner bookkeeping).
func (t *Transcript) HasTurn(turnID int64) bool {
	_, ok := t.byTurn[turnID]
	return ok
}

// Len returns the total event count.
func (t *Transcript) Len() int {
	return len(t.byEvent)
}

// View renders the full scrollback at width, re-rendering only dirty
// turns. Width changes invalidate everything.
func (t *Transcript) View(width int) string {
	if width != t.width {
		t.width = width
		t.dirtyAll()
	}
	if t.joinedOK {
		return t.joined
	}
	var b strings.Builder
	for i, turn := range t.turns {
		if turn.dirty {
			turn.rendered = t.renderTurn(turn)
			turn.dirty = false
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(turn.rendered)
	}
	t.joined = b.String()
	t.joinedOK = true
	return t.joined
}

func (t *Transcript) renderTurn(turn *Turn) string {
	var b strings.Builder
	for _, ev := range turn.Events {
		b.WriteString(t.render(ev, t.width))
	}
	return b.String()
}

func (t *Transcript) dirtyAll() {
	for _, turn := range t.turns {
		turn.dirty = true
	}
	t.joinedOK = false
}
