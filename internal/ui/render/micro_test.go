package render

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// ── Effect 1: error shake ────────────────────────────────────────────────

// applyShake's own pure contract: offset <= 0 is a no-op (byte-identical
// return), offset > 0 pads every non-blank line and leaves blank lines
// (the trailing element after a block's own final "\n") untouched.
func TestApplyShakeOffset(t *testing.T) {
	block := "abc\ndef\n"
	if got := applyShake(block, 0); got != block {
		t.Errorf("offset 0 must no-op: got %q, want %q", got, block)
	}
	if got := applyShake(block, -3); got != block {
		t.Errorf("negative offset must no-op: got %q, want %q", got, block)
	}
	want := "  abc\n  def\n"
	if got := applyShake(block, 2); got != want {
		t.Errorf("applyShake(_, 2) = %q, want %q", got, want)
	}
	// A single trailing blank line stays blank rather than gaining
	// invisible trailing whitespace.
	if got := applyShake("only\n", 1); got != " only\n" {
		t.Errorf("applyShake(%q, 1) = %q, want %q", "only\n", got, " only\n")
	}
}

// SetShake keys strictly by event ID: an event with an entry in the map
// renders left-padded; every other event — including one rendered by the
// SAME renderer moments later — renders at its resting position.
func TestShakeAppliesOnlyToItsOwnEvent(t *testing.T) {
	r := plain()
	r.SetShake(map[int64]int{5: 2})

	shaking := stripANSI(r.Event(api.Event{ID: 5, TurnID: 5, Kind: api.KindError, Payload: json.RawMessage(`{"text":"boom"}`)}))
	if !strings.HasPrefix(shaking, "  ┃") {
		t.Errorf("event 5 (mid-shake) must render left-padded: %q", shaking)
	}

	settled := stripANSI(r.Event(api.Event{ID: 6, TurnID: 6, Kind: api.KindError, Payload: json.RawMessage(`{"text":"boom"}`)}))
	if !strings.HasPrefix(settled, "┃") {
		t.Errorf("event 6 (no shake entry) must render at rest: %q", settled)
	}

	// Clearing the map (the shake settling out, micro.go's onAnimTick)
	// returns event 5 to its resting position too — the offset is live
	// state, not sticky per-event styling.
	r.SetShake(nil)
	atRest := stripANSI(r.Event(api.Event{ID: 5, TurnID: 5, Kind: api.KindError, Payload: json.RawMessage(`{"text":"boom"}`)}))
	if !strings.HasPrefix(atRest, "┃") {
		t.Errorf("event 5 after SetShake(nil) must render at rest: %q", atRest)
	}
}

// ── Effect 2: confirm glint ──────────────────────────────────────────────

func TestConfirmGlintRow(t *testing.T) {
	cases := []struct {
		progress float64
		rows     int
		want     int
	}{
		{0, 5, 0},
		{1, 5, 4},
		{0.5, 5, 2},
		{0, 1, 0},
		{1, 1, 0},
		{0, 0, -1},
		{0.5, 0, -1},
	}
	for _, tc := range cases {
		if got := confirmGlintRow(tc.progress, tc.rows); got != tc.want {
			t.Errorf("confirmGlintRow(%v, %d) = %d, want %d", tc.progress, tc.rows, got, tc.want)
		}
	}
}

func TestBrighten(t *testing.T) {
	got := brighten(RGB{R: 100, G: 100, B: 100}, 0.4)
	want := RGB{R: 162, G: 162, B: 162} // 100 + (255-100)*0.4 = 162
	if got != want {
		t.Errorf("brighten(100,0.4) = %+v, want %+v", got, want)
	}
	if got := brighten(RGB{R: 255, G: 255, B: 255}, 0.4); got != (RGB{R: 255, G: 255, B: 255}) {
		t.Errorf("brighten must clamp at 255: got %+v", got)
	}
	if got := brighten(RGB{R: 10, G: 10, B: 10}, 0); got != (RGB{R: 10, G: 10, B: 10}) {
		t.Errorf("amount 0 must no-op: got %+v", got)
	}
}

// confirmChrome must recolor EXACTLY the glint's own row — every other
// row keeps the shared pulseWarn base, and with no sweep live (glint <
// 0, New's own default) every row matches.
func TestConfirmChromeGlintRecolorsExactlyOneRow(t *testing.T) {
	r := plain()
	r.phase = 0
	content := "line one\nline two\nline three"

	borderColor := func(line string) string {
		i := strings.Index(line, "┃")
		if i < 0 {
			return ""
		}
		return line[:i]
	}

	r.glint = -1
	settled := r.confirmChrome(content)
	lines := strings.Split(settled, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), settled)
	}
	base := borderColor(lines[0])
	for i, line := range lines {
		if c := borderColor(line); c != base {
			t.Errorf("no sweep live: row %d border color = %q, want %q", i, c, base)
		}
	}

	r.glint = 0 // sweep sitting at its very first row
	lit := r.confirmChrome(content)
	litLines := strings.Split(lit, "\n")
	if borderColor(litLines[0]) == base {
		t.Error("row 0 must recolor once the glint sweep sits at progress 0")
	}
	for i := 1; i < len(litLines); i++ {
		if c := borderColor(litLines[i]); c != base {
			t.Errorf("only the glint's own row may recolor — row %d changed too: %q", i, c)
		}
	}

	r.glint = 1 // sweep at its last row
	last := r.confirmChrome(content)
	lastLines := strings.Split(last, "\n")
	if borderColor(lastLines[2]) == base {
		t.Error("the last row must recolor once the glint sweep sits at progress 1")
	}
	if borderColor(lastLines[0]) != base {
		t.Error("row 0 must be back to the shared base once the sweep has moved on")
	}
}
