package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// countingRenderer renders "kind:payload" one-liners and counts invocations
// so caching behavior is observable.
type countingRenderer struct{ calls int }

func (c *countingRenderer) render(ev api.Event, _ int) string {
	c.calls++
	return fmt.Sprintf("[%d/%d %s %s]\n", ev.TurnID, ev.ID, ev.Kind, string(ev.Payload))
}

func ev(id, turnID int64, kind, payload string) api.Event {
	return api.Event{ID: id, TurnID: turnID, Kind: kind, Payload: json.RawMessage(payload)}
}

func TestAppendGroupsByTurnInArrivalOrder(t *testing.T) {
	c := &countingRenderer{}
	tr := NewTranscript(c.render)
	tr.Append(ev(1, 1, "echo", `{}`))
	tr.Append(ev(2, 1, "system", `{}`))
	tr.Append(ev(3, 2, "echo", `{}`))

	view := tr.View(80)
	want := "[1/1 echo {}]\n\n[1/2 system {}]\n\n[2/3 echo {}]\n"
	if view != want {
		t.Errorf("view =\n%q\nwant\n%q", view, want)
	}
}

// pev is ev plus the server's per-conversation position — the field that
// orders a turn when cable and re-sync deliveries interleave.
func pev(id, turnID, position int64, kind, payload string) api.Event {
	e := ev(id, turnID, kind, payload)
	e.Position = position
	return e
}

// The stacked-indicator repro (owner screenshot 2026-07-17 02:01, starved
// prod): the cable dropped a turn's echo in a reconnect's subscribe-confirm
// gap, delivered the turn's thinking indicator live, and the re-sync Merge
// recovered the echo AFTERWARD — so the indicator rendered above its own
// echo and read as a second spinner stacked on the previous turn. Position
// must order the turn, not arrival.
func TestAppendSlotsLateMergedEventByPosition(t *testing.T) {
	c := &countingRenderer{}
	tr := NewTranscript(c.render)
	// Previous turn, fully delivered: echo + its pending indicator.
	tr.Append(pev(1, 1, 1, "echo", `{"text":"show me games with hard bosses"}`))
	tr.Append(pev(2, 1, 2, "thinking", `{"resolved":false}`))
	// Next turn: the indicator arrives live FIRST (echo broadcast missed),
	// then the re-sync merge recovers the echo.
	tr.Append(pev(4, 2, 4, "thinking", `{"resolved":false}`))
	tr.Merge([]api.Event{pev(3, 2, 3, "echo", `{"text":"show me hard games"}`)})

	view := tr.View(80)
	echoAt := strings.Index(view, "[2/3 echo")
	thinkAt := strings.Index(view, "[2/4 thinking")
	if echoAt == -1 || thinkAt == -1 {
		t.Fatalf("missing events in view:\n%s", view)
	}
	if echoAt > thinkAt {
		t.Errorf("turn 2's indicator rendered above its own echo (the stacked-indicator bug):\n%s", view)
	}
	// Exactly ONE indicator line per pending turn — never two in one block.
	if got := strings.Count(view, "thinking"); got != 2 {
		t.Errorf("thinking lines = %d, want 2 (one per pending turn):\n%s", got, view)
	}
}

// The turn's resolve (event.replace, resolved: true) must still land on the
// re-slotted indicator: after the out-of-order recovery above, the replace
// swaps the spinner for the timing summary in place, below the echo.
func TestReplaceResolvesReSlottedThinkingInPlace(t *testing.T) {
	tr := NewTranscript((&countingRenderer{}).render)
	tr.Append(pev(4, 2, 4, "thinking", `{"resolved":false}`))
	tr.Merge([]api.Event{pev(3, 2, 3, "echo", `{"text":"show me hard games"}`)})
	tr.Replace(pev(4, 2, 4, "thinking", `{"resolved":true,"elapsed_seconds":193.96}`))

	view := tr.View(80)
	if strings.Contains(view, `"resolved":false`) {
		t.Errorf("pending indicator survived its resolve:\n%s", view)
	}
	echoAt := strings.Index(view, "[2/3 echo")
	doneAt := strings.Index(view, `193.96`)
	if doneAt == -1 || echoAt == -1 || doneAt < echoAt {
		t.Errorf("resolved summary must render once, below the echo:\n%s", view)
	}
}

// Position-less events (synthetic locals, servers predating the field) keep
// plain arrival order — the pre-Position behavior, byte-identical.
func TestAppendWithoutPositionsKeepsArrivalOrder(t *testing.T) {
	tr := NewTranscript((&countingRenderer{}).render)
	tr.Append(ev(2, 1, "system", `{}`))
	tr.Append(ev(1, 1, "echo", `{}`)) // lower ID, no position: stays second
	view := tr.View(80)
	if strings.Index(view, "[1/2 system") > strings.Index(view, "[1/1 echo") {
		t.Errorf("position-less events must keep arrival order:\n%s", view)
	}
}

// A mid-turn insert shifts later slots; byEvent must track them so a
// replace (or payload mutation) still targets the right event.
func TestMidTurnInsertKeepsEventIndexCoherent(t *testing.T) {
	tr := NewTranscript((&countingRenderer{}).render)
	tr.Append(pev(4, 2, 4, "thinking", `{"resolved":false}`))
	tr.Append(pev(3, 2, 3, "echo", `{"text":"late echo"}`)) // inserts BEFORE the indicator
	tr.Replace(pev(4, 2, 4, "thinking", `{"resolved":true}`))

	view := tr.View(80)
	if !strings.Contains(view, `[2/4 thinking {"resolved":true}`) {
		t.Errorf("replace after a mid-turn insert hit the wrong slot:\n%s", view)
	}
	if !strings.Contains(view, "late echo") {
		t.Errorf("inserted echo lost:\n%s", view)
	}
}

func TestDuplicateAppendIsNoOp(t *testing.T) {
	tr := NewTranscript((&countingRenderer{}).render)
	tr.Append(ev(1, 1, "echo", `{"v":1}`))
	tr.Append(ev(1, 1, "echo", `{"v":2}`)) // same ID: dropped
	if tr.Len() != 1 {
		t.Fatalf("Len = %d, want 1", tr.Len())
	}
	if !strings.Contains(tr.View(80), `{"v":1}`) {
		t.Error("duplicate overwrote the original")
	}
}

func TestReplaceRewritesInPlace(t *testing.T) {
	tr := NewTranscript((&countingRenderer{}).render)
	tr.Append(ev(1, 1, "confirmation", `{"state":"pending"}`))
	tr.Append(ev(2, 1, "system", `{}`))
	tr.Replace(ev(1, 1, "confirmation", `{"state":"resolved"}`))

	view := tr.View(80)
	if !strings.Contains(view, `resolved`) || strings.Contains(view, `pending`) {
		t.Errorf("replace did not rewrite in place: %q", view)
	}
	// Position preserved: the replaced event still renders before event 2.
	if strings.Index(view, "resolved") > strings.Index(view, "[1/2") {
		t.Errorf("replaced event moved: %q", view)
	}
}

func TestReplaceUnseenIDAppendsDefensively(t *testing.T) {
	tr := NewTranscript((&countingRenderer{}).render)
	tr.Replace(ev(9, 3, "system", `{}`))
	if tr.Len() != 1 || !tr.HasTurn(3) {
		t.Error("replace of unseen ID must append")
	}
}

func TestDirtyCachingRerendersOnlyTouchedTurns(t *testing.T) {
	c := &countingRenderer{}
	tr := NewTranscript(c.render)
	tr.Append(ev(1, 1, "echo", `{}`))
	tr.Append(ev(2, 2, "echo", `{}`))
	tr.Append(ev(3, 3, "echo", `{}`))

	tr.View(80)
	if c.calls != 3 {
		t.Fatalf("initial render calls = %d, want 3", c.calls)
	}
	tr.View(80) // cached
	if c.calls != 3 {
		t.Errorf("cached view re-rendered: calls = %d", c.calls)
	}

	tr.Replace(ev(2, 2, "echo", `{"new":true}`))
	tr.View(80)
	if c.calls != 4 {
		t.Errorf("replace must re-render exactly one turn: calls = %d, want 4", c.calls)
	}
}

func TestWidthChangeRerendersEverything(t *testing.T) {
	c := &countingRenderer{}
	tr := NewTranscript(c.render)
	tr.Append(ev(1, 1, "echo", `{}`))
	tr.Append(ev(2, 2, "echo", `{}`))
	tr.View(80)
	tr.View(40)
	if c.calls != 4 {
		t.Errorf("resize must dirty all turns: calls = %d, want 4", c.calls)
	}
}

func TestSetRendererInvalidatesCache(t *testing.T) {
	c1 := &countingRenderer{}
	tr := NewTranscript(c1.render)
	tr.Append(ev(1, 1, "echo", `{}`))
	tr.View(80)

	c2 := &countingRenderer{}
	tr.SetRenderer(c2.render)
	tr.View(80)
	if c2.calls != 1 {
		t.Errorf("new renderer calls = %d, want 1", c2.calls)
	}
}

func TestMergeIsIdempotentAndDetectsChanges(t *testing.T) {
	tr := NewTranscript((&countingRenderer{}).render)
	initial := []api.Event{
		ev(1, 1, "echo", `{"text":"hi"}`),
		ev(2, 1, "system", `{"text":"hello"}`),
	}
	if changed := tr.Merge(initial); changed != 2 {
		t.Errorf("initial merge changed = %d, want 2", changed)
	}
	if changed := tr.Merge(initial); changed != 0 {
		t.Errorf("identical merge changed = %d, want 0", changed)
	}

	// Offline gap: event 2 was replaced server-side, event 3 appended.
	after := []api.Event{
		ev(1, 1, "echo", `{"text":"hi"}`),
		ev(2, 1, "system", `{"text":"edited"}`),
		ev(3, 2, "echo", `{"text":"while offline"}`),
	}
	if changed := tr.Merge(after); changed != 2 {
		t.Errorf("resync merge changed = %d, want 2", changed)
	}
	view := tr.View(80)
	if !strings.Contains(view, "edited") || !strings.Contains(view, "while offline") {
		t.Errorf("merge lost data: %q", view)
	}
}

func TestEventLineRangeMatchesOwningTurn(t *testing.T) {
	c := &countingRenderer{}
	tr := NewTranscript(c.render)
	tr.Append(ev(1, 1, "echo", `{}`))
	tr.Append(ev(2, 2, "echo", `{}`))
	tr.Append(ev(3, 3, "echo", `{}`))
	tr.View(80) // render so every turn's line cache is current

	wantStart, wantEnd, wantOK := tr.TurnLineRange(2)
	if !wantOK {
		t.Fatal("TurnLineRange(2) must be ok once rendered")
	}
	gotStart, gotEnd, gotOK := tr.EventLineRange(2) // event 2 belongs to the middle turn
	if !gotOK || gotStart != wantStart || gotEnd != wantEnd {
		t.Errorf("EventLineRange(2) = (%d, %d, %v), want (%d, %d, true) matching TurnLineRange(2)",
			gotStart, gotEnd, gotOK, wantStart, wantEnd)
	}
}

func TestEventLineRangeUnknownEventIsNotOK(t *testing.T) {
	c := &countingRenderer{}
	tr := NewTranscript(c.render)
	tr.Append(ev(1, 1, "echo", `{}`))
	tr.View(80)

	if _, _, ok := tr.EventLineRange(999); ok {
		t.Error("EventLineRange for an unknown event id must be ok=false")
	}
}

func TestEventLineRangeUnrenderedTurnIsNotOK(t *testing.T) {
	c := &countingRenderer{}
	tr := NewTranscript(c.render)
	tr.Append(ev(1, 1, "echo", `{}`)) // never rendered at any width — turn stays dirty

	if _, _, ok := tr.EventLineRange(1); ok {
		t.Error("EventLineRange for an unrendered (dirty) turn must be ok=false, inheriting TurnLineRange's contract")
	}
}
