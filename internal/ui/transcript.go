package ui

import (
	"bytes"
	"encoding/json"
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
	lines    []string // rendered, split once — the window join's currency
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
//
// WITHIN a turn the event slots by the server's Position, not by arrival:
// arrival order lies exactly when it matters most. A broadcast missed in a
// reconnect's subscribe-confirm gap is recovered by the re-sync Merge —
// AFTER the cable already delivered the turn's later events live (owner
// screenshot 2026-07-17 02:01, starved prod: a turn's echo merged in under
// its own thinking indicator, so the spinner rendered above its echo and
// read as a second indicator stacked on the PREVIOUS turn). Position-less
// events (0 — synthetic locals, servers predating the field) keep plain
// arrival order.
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
	idx := turn.insertionIndex(ev)
	turn.Events = append(turn.Events, api.Event{})
	copy(turn.Events[idx+1:], turn.Events[idx:])
	turn.Events[idx] = ev
	// Reindex the shifted tail (byEvent carries each event's slot).
	for i := idx; i < len(turn.Events); i++ {
		t.byEvent[turn.Events[i].ID] = eventPos{turn: turn, idx: i}
	}
	turn.dirty = true
	t.joinedOK = false
}

// insertionIndex finds where ev belongs in the turn: after the last event
// that is not provably later than it. Only KNOWN positions (> 0) ever hop
// backward over other known positions — an unknown (0) stops the walk, so
// runs of position-less events are never reordered and the pre-Position
// behavior (plain append) is byte-identical when nothing carries one.
func (turn *Turn) insertionIndex(ev api.Event) int {
	if ev.Position <= 0 {
		return len(turn.Events)
	}
	for i := len(turn.Events) - 1; i >= 0; i-- {
		if p := turn.Events[i].Position; p <= 0 || p <= ev.Position {
			return i + 1
		}
	}
	return 0
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

// MutateEventPayload looks up an event by ID and rewrites its Payload via
// fn, marking the owning turn dirty — the ai_block/ai_status streaming
// mutations (model.go) need to edit one event's payload bytes in place
// without walking Append/Replace's turn-management logic. fn returning
// ok=false (a decode failure, or the caller declining to touch the
// payload) leaves the event untouched. An unknown eventID is a silent
// no-op — an ai_block/ai_status racing ahead of its event.append, which
// the eventual event.replace reconciles either way.
func (t *Transcript) MutateEventPayload(eventID int64, fn func(payload json.RawMessage) (json.RawMessage, bool)) (api.Event, bool) {
	pos, known := t.byEvent[eventID]
	if !known {
		return api.Event{}, false
	}
	next, ok := fn(pos.turn.Events[pos.idx].Payload)
	if !ok {
		return api.Event{}, false
	}
	pos.turn.Events[pos.idx].Payload = next
	pos.turn.dirty = true
	t.joinedOK = false
	return pos.turn.Events[pos.idx], true
}

// Touch marks one turn dirty (shimmer animation re-renders it per tick
// without invalidating the rest of the cache).
func (t *Transcript) Touch(turnID int64) {
	if turn, ok := t.byTurn[turnID]; ok {
		turn.dirty = true
		t.joinedOK = false
	}
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
	if t.joinedOK && width == t.width {
		return t.joined
	}
	t.ensureRendered(width)
	var b strings.Builder
	for i, turn := range t.turns {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(turn.rendered)
	}
	t.joined = b.String()
	t.joinedOK = true
	return t.joined
}

// ensureRendered brings every dirty turn's cache (rendered + lines)
// current at the given width. The full pass runs once per data/width
// change per turn — animation ticks only dirty VISIBLE turns (model.go's
// visible-only Touch), so a long scrollback costs nothing per frame.
func (t *Transcript) ensureRendered(width int) {
	if width != t.width {
		t.width = width
		t.dirtyAll()
	}
	for _, turn := range t.turns {
		if turn.dirty {
			turn.rendered = t.renderTurn(turn)
			turn.lines = strings.Split(turn.rendered, "\n")
			turn.dirty = false
		}
	}
}

// TotalLines is the joined transcript's height in lines at width —
// turns join with a single newline, so it is simply the sum of each
// turn's own line count (the boundary newline is the seam between two
// turns' edge lines, not an extra line).
func (t *Transcript) TotalLines(width int) int {
	t.ensureRendered(width)
	total := 0
	for _, turn := range t.turns {
		total += len(turn.lines)
	}
	return total
}

// WindowLines returns lines [yoff, yoff+height) of the joined transcript
// WITHOUT joining the whole thing — the virtualized viewport's core
// (owner 2026-07-12: long conversations lagged because every frame
// re-joined and re-measured the entire scrollback; the web renders only
// what's visible). O(visible) per call once caches are warm.
func (t *Transcript) WindowLines(width, yoff, height int) []string {
	t.ensureRendered(width)
	if yoff < 0 {
		yoff = 0
	}
	out := make([]string, 0, height)
	pos := 0
	for _, turn := range t.turns {
		n := len(turn.lines)
		if pos+n <= yoff {
			pos += n
			continue
		}
		start := 0
		if yoff > pos {
			start = yoff - pos
		}
		for i := start; i < n && len(out) < height; i++ {
			out = append(out, turn.lines[i])
		}
		pos += n
		if len(out) >= height {
			break
		}
	}
	return out
}

// TurnLineRange reports [start, end) line positions of a turn within the
// joined transcript, ok=false when absent or not yet rendered — the
// visible-only animation gate's lookup (model.go touches only shimmer
// turns that intersect the viewport window).
func (t *Transcript) TurnLineRange(turnID int64) (start, end int, ok bool) {
	pos := 0
	for _, turn := range t.turns {
		n := len(turn.lines)
		if turn.ID == turnID {
			if turn.dirty && n == 0 {
				return 0, 0, false
			}
			return pos, pos + n, true
		}
		pos += n
	}
	return 0, 0, false
}

// EventLineRange reports [start, end) line positions of the turn that owns
// eventID within the joined transcript, ok=false when the event id is
// unknown (resolving it via byEvent) or its turn isn't rendered yet — the
// jump-to-first-mention primitive for conversation search (pito-tui 3.0.0
// U2.1): a hit card carries an anchor event id, and this turns it into a
// scroll offset for SetYOffset.
func (t *Transcript) EventLineRange(eventID int64) (start, end int, ok bool) {
	pos, known := t.byEvent[eventID]
	if !known {
		return 0, 0, false
	}
	return t.TurnLineRange(pos.turn.ID)
}

// LatestEvent returns the most recently appended event — the last event of
// the last turn — ok=false when the transcript is empty. The
// conversation-hits jump affordance's gate (hitpicker.go): only the NEWEST
// rendered event's payload is checked for conversation_hits, mirroring
// LiveHandles' "the newest turn wins" rule but at event granularity — a
// hits card is always the sole event of its own turn, so the last turn's
// last event is exactly the card, when there is one.
func (t *Transcript) LatestEvent() (api.Event, bool) {
	if len(t.turns) == 0 {
		return api.Event{}, false
	}
	turn := t.turns[len(t.turns)-1]
	if len(turn.Events) == 0 {
		return api.Event{}, false
	}
	return turn.Events[len(turn.Events)-1], true
}

// TurnsOutside counts turns FULLY above and FULLY below the viewport
// window [yoff, yoff+height) — the scroll-nav pills' numbers (the web's
// scroll_nav_controller counts [data-scrollback-message] rects the same
// way; a turn straddling an edge belongs to neither count). Line ranges
// come from the same caches TurnLineRange walks, materialized for width.
func (t *Transcript) TurnsOutside(width, yoff, height int) (above, below int) {
	t.ensureRendered(width)
	pos := 0
	for _, turn := range t.turns {
		n := len(turn.lines)
		if pos+n <= yoff && n > 0 {
			above++
		} else if pos >= yoff+height && n > 0 {
			below++
		}
		pos += n
	}
	return above, below
}

func (t *Transcript) renderTurn(turn *Turn) string {
	// (lines is refreshed by ensureRendered's caller-side split.)
	var b strings.Builder
	for i, ev := range turn.Events {
		if i > 0 {
			b.WriteString("\n")
		}
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

// LiveHandle pairs a still-actionable reply handle with its event's kind —
// ai handles prefill "@ai " continuations (2.0.0), everything else replies
// bare.
// TurnEvents returns the events of one turn (nil when absent) — the
// shimmer bookkeeping's window into a turn when deciding whether it
// still needs animation ticks after a replace.
func (t *Transcript) TurnEvents(turnID int64) []api.Event {
	if turn, ok := t.byTurn[turnID]; ok {
		return turn.Events
	}
	return nil
}

// EchoTexts returns the user's sent inputs newest-first, read from the
// echo events currently in the transcript — the TUI analog of the web
// seeding its input history from the conversation's last 50 turns
// (conversations/show.html.erb's sent_history). Consecutive duplicates
// collapse and the list caps at limit, mirroring history_controller.js's
// own dedupe/cap rules so a reload reproduces what a live session built.
func (t *Transcript) EchoTexts(limit int) []string {
	var out []string
	for i := len(t.turns) - 1; i >= 0 && len(out) < limit; i-- {
		for _, ev := range t.turns[i].Events {
			if ev.Kind != api.KindEcho {
				continue
			}
			var p struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(ev.Payload, &p) != nil || strings.TrimSpace(p.Text) == "" {
				continue
			}
			text := strings.TrimSpace(p.Text)
			if len(out) > 0 && out[len(out)-1] == text {
				continue
			}
			out = append(out, text)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

type LiveHandle struct {
	Handle string
	Kind   string
}

// LiveHandles returns the reply handles that are still actionable: the
// NEWEST turn carrying any non-consumed reply_handle wins outright — the
// server retires all prior live hashtags whenever a new leading turn
// arrives (finalizer sweep), so older turns' handles are dead anyway.
func (t *Transcript) LiveHandles() []LiveHandle {
	for i := len(t.turns) - 1; i >= 0; i-- {
		var handles []LiveHandle
		for _, ev := range t.turns[i].Events {
			var p struct {
				ReplyHandle   string `json:"reply_handle"`
				ReplyConsumed bool   `json:"reply_consumed"`
			}
			if json.Unmarshal(ev.Payload, &p) != nil {
				continue
			}
			if p.ReplyHandle != "" && !p.ReplyConsumed {
				handles = append(handles, LiveHandle{Handle: p.ReplyHandle, Kind: ev.Kind})
			}
		}
		if len(handles) > 0 {
			return handles
		}
	}
	return nil
}
