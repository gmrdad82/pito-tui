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
	want := "[1/1 echo {}]\n[1/2 system {}]\n\n[2/3 echo {}]\n"
	if view != want {
		t.Errorf("view =\n%q\nwant\n%q", view, want)
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
