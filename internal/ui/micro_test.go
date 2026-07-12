package ui

import (
	"net/http"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
)

// ── Effect 1: error shake ────────────────────────────────────────────────

// The jitter's own fixed sequence: exact for its ticks, 0 for anything
// out of range (stretched ×2 for the 60fps loop — same ~160ms shape).
func TestShakeOffsetSequenceExactThenZero(t *testing.T) {
	want := []int{1, 1, -1, -1, 1, 1, 0, 0, 0, 0}
	for i, w := range want {
		if got := shakeOffset(i); got != w {
			t.Errorf("shakeOffset(%d) = %d, want %d", i, got, w)
		}
	}
	for _, tick := range []int{-1, len(errorShakeOffsets), len(errorShakeOffsets) + 10} {
		if got := shakeOffset(tick); got != 0 {
			t.Errorf("shakeOffset(%d) out of range = %d, want 0", tick, got)
		}
	}
}

// End to end: an error event landing on the transcript starts its own
// shake immediately (visible on the very FIRST render, no tick lag),
// ticks through errorShakeOffsets' exact sequence, then drops out of the
// map and settles the gate — and never touches a sibling event's own
// block.
func TestErrorShakeModelFlowTicksThenSettles(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true))
	m = sized(m)
	m = drive(m,
		CableEventMsg{M: cable.StreamMessage{
			Type:  cable.TypeEventAppend,
			Event: api.Event{ID: 8, TurnID: 9, Kind: "system", Payload: []byte(`{"text":"all fine"}`)},
		}},
		CableEventMsg{M: cable.StreamMessage{
			Type:  cable.TypeEventAppend,
			Event: api.Event{ID: 9, TurnID: 9, Kind: api.KindError, Payload: []byte(`{"text":"boom"}`)},
		}},
	)

	sh, ok := m.shaking[9]
	if !ok || sh.tick != 0 {
		t.Fatalf("appending an error event must start its own shake at tick 0: %+v ok=%v", sh, ok)
	}
	if !m.animGateOpen() {
		t.Fatal("an in-flight shake must hold the gate open")
	}

	// pushShakeOffsets runs synchronously right after beginErrorShake, so
	// tick 0's offset is already visible before any AnimTickMsg fires.
	view := ansi.Strip(m.viewContent())
	if !strings.Contains(view, " ┃ ✗ boom") {
		t.Errorf("tick 0's offset must already be visible on the very first render:\n%s", view)
	}
	if strings.Contains(view, " ┃ all fine") {
		t.Errorf("the shake must never touch the sibling event's own block:\n%s", view)
	}

	var ticks []int
	for step := 0; step < len(errorShakeOffsets)+2; step++ {
		if len(m.shaking) == 0 {
			break
		}
		m = drive(m, AnimTickMsg{})
		if sh, ok := m.shaking[9]; ok {
			ticks = append(ticks, sh.tick)
		}
	}
	wantTicks := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
	if len(ticks) != len(wantTicks) {
		t.Fatalf("tick sequence = %v, want %v", ticks, wantTicks)
	}
	for i, w := range wantTicks {
		if ticks[i] != w {
			t.Errorf("tick[%d] = %d, want %d (full sequence %v)", i, ticks[i], w, ticks)
		}
	}
	if len(m.shaking) != 0 {
		t.Errorf("shake must be gone from the map once its sequence completes: %+v", m.shaking)
	}
	// The error's cable delivery also armed the dot's activity pulse
	// (ambient.go effect 3) — breathe it out before asserting the gate.
	for m.dotPulseTicks > 0 {
		next, _ := m.Update(AnimTickMsg{})
		m = next.(Model)
	}
	if m.animGateOpen() {
		t.Error("the gate must close once the shake settles (nothing else pending)")
	}
	if settled := ansi.Strip(m.viewContent()); strings.Contains(settled, " ┃ ✗ boom") {
		t.Errorf("a settled event must render at rest (no pad):\n%s", settled)
	}
}

// ── Effect 2: confirm glint ──────────────────────────────────────────────

func TestConfirmGlintProgressCycleMath(t *testing.T) {
	if got := confirmGlintProgress(0); got != 0 {
		t.Errorf("progress at tick 0 = %v, want 0", got)
	}
	if got := confirmGlintProgress(glintWindowTicks - 1); got != 1 {
		t.Errorf("progress at the window's own last tick = %v, want 1", got)
	}
	if got := confirmGlintProgress(glintWindowTicks); got != -1 {
		t.Errorf("progress just past the window must be inactive (-1), got %v", got)
	}
	if got := confirmGlintProgress(glintCycleTicks - 1); got != -1 {
		t.Errorf("progress just before the cycle wraps must be inactive, got %v", got)
	}
	if got := confirmGlintProgress(glintCycleTicks); got != 0 {
		t.Errorf("the cycle must repeat identically: got %v, want 0", got)
	}
	prev := -2.0
	for tick := int64(0); tick < glintWindowTicks; tick++ {
		got := confirmGlintProgress(tick)
		if got < prev {
			t.Fatalf("progress must never regress inside the window: tick %d = %v after %v", tick, got, prev)
		}
		prev = got
	}
}

// A pending confirmation already holds the fast gate open via its own
// m.shimmer entry (existing wiring) — the glint needs no dedicated gate
// entry of its own, riding m.aliveTicks for free across a whole cycle.
func TestConfirmGlintRidesAliveTicksWithoutOwnGateEntry(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true))
	m = sized(m)
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type: cable.TypeEventAppend,
		Event: api.Event{ID: 5, TurnID: 5, Kind: api.KindConfirmation, Payload: []byte(
			`{"body":"Unlink Hades II?","reply_handle":"@confirm-1"}`)},
	}})
	if len(m.shimmer) == 0 {
		t.Fatal("a pending confirmation must hold m.shimmer (and therefore the gate) open on its own")
	}
	for i := int64(0); i < glintCycleTicks+glintWindowTicks; i++ {
		if !m.animGateOpen() {
			t.Fatalf("gate closed early at tick %d — a still-pending confirmation must keep it open", i)
		}
		m = drive(m, AnimTickMsg{})
	}
}

// ── Effect 3: ghost typing ───────────────────────────────────────────────

func TestGhostRevealedTextSliceProgression(t *testing.T) {
	target := "next: 28d"
	if got := ghostRevealedText(target, 0); got != "" {
		t.Errorf("tick 0 must be empty, got %q", got)
	}
	if got := ghostRevealedText(target, -1); got != "" {
		t.Errorf("negative tick must be empty, got %q", got)
	}
	prevLen := 0
	for tick := int64(1); tick < ghostTypeTicks; tick++ {
		got := ghostRevealedText(target, tick)
		if !strings.HasPrefix(target, got) {
			t.Fatalf("tick %d must be a PREFIX of target: got %q", tick, got)
		}
		if n := len([]rune(got)); n < prevLen {
			t.Fatalf("reveal must never regress: tick %d = %q (len %d) after len %d", tick, got, n, prevLen)
		} else {
			prevLen = n
		}
	}
	if got := ghostRevealedText(target, ghostTypeTicks); got != target {
		t.Errorf("tick == ghostTypeTicks must be the full string, got %q", got)
	}
	if got := ghostRevealedText(target, ghostTypeTicks+10); got != target {
		t.Errorf("tick past the budget must clamp to the full string, got %q", got)
	}
	if got := ghostRevealedText("", 2); got != "" {
		t.Errorf("empty target must stay empty regardless of tick, got %q", got)
	}
}

// End to end: a fresh Ghost.NextHint types itself in over ghostTypeTicks,
// then settles; a repeat of the SAME hint text never restarts it.
func TestGhostTypeModelFlowRevealsThenSettles(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true))
	m = sized(m)
	m.input.SetValue("ls vids")
	const hint = "shift+tab next channel"
	newSuggest := func() *api.Suggestions {
		return &api.Suggestions{
			MenuItems: []api.Suggestion{{Label: "vids"}},
			Ghost:     api.Ghost{NextHint: hint},
		}
	}

	m = drive(m, SuggestionsMsg{Seq: m.suggestSeq, S: newSuggest()})
	if !m.ghostTyping || m.ghostTarget != hint {
		t.Fatalf("a fresh hint must start its own type-in: typing=%v target=%q", m.ghostTyping, m.ghostTarget)
	}
	if !m.animGateOpen() {
		t.Fatal("an in-flight type-in must hold the gate open")
	}
	if shown := ansi.Strip(m.paletteView()); strings.Contains(shown, hint) {
		t.Errorf("the full hint must not appear before any tick: %q", shown)
	}

	for i := int64(0); i < ghostTypeTicks && m.ghostTyping; i++ {
		m = drive(m, AnimTickMsg{})
	}
	if m.ghostTyping {
		t.Error("the type-in must settle within its own ~150ms budget")
	}
	if m.animGateOpen() {
		t.Error("the gate must close once the type-in settles (nothing else pending)")
	}
	if got := ansi.Strip(m.paletteView()); !strings.Contains(got, hint) {
		t.Errorf("the settled palette must show the full hint: %q", got)
	}

	// An unchanged hint arriving again must not restart the reveal.
	m = drive(m, SuggestionsMsg{Seq: m.suggestSeq, S: newSuggest()})
	if m.ghostTyping {
		t.Error("an unchanged hint must not restart the type-in")
	}
}

// ── Effect 4: scroll thumb ───────────────────────────────────────────────

func TestThumbGeometryFractionToRows(t *testing.T) {
	top, rows := thumbGeometry(20, 5, 20, 0)
	if rows != 5 {
		t.Errorf("thumbRows = %d, want 5 (20*5/20)", rows)
	}
	if top != 0 {
		t.Errorf("percent 0 must pin the thumb to the track's top, got %d", top)
	}
	top, rows = thumbGeometry(20, 5, 20, 1)
	if top != 20-rows {
		t.Errorf("percent 1 must pin the thumb to the track's bottom: top=%d rows=%d", top, rows)
	}
	if _, rows := thumbGeometry(20, 1, 10000, 0.5); rows != 1 {
		t.Errorf("thumbRows must floor at 1 for a huge scrollback, got %d", rows)
	}
	if top, rows := thumbGeometry(0, 5, 20, 0.5); top != 0 || rows != 0 {
		t.Errorf("zero track height must report nothing: top=%d rows=%d", top, rows)
	}
	if top, rows := thumbGeometry(20, 5, 0, 0.5); top != 0 || rows != 0 {
		t.Errorf("zero total lines must report nothing: top=%d rows=%d", top, rows)
	}
}

// setFollow is the ONE seam that starts/cancels the fade — a pure,
// map-free unit test of that bookkeeping in isolation.
func TestSetFollowStartsAndCancelsFade(t *testing.T) {
	m := Model{truecolor: true, follow: false}
	m.setFollow(true)
	if !m.thumbFading || m.thumbFadeTick != 0 {
		t.Fatalf("false→true must start the fade from tick 0: fading=%v tick=%d", m.thumbFading, m.thumbFadeTick)
	}
	m.thumbFadeTick = 5
	m.setFollow(true) // already true — must stay a no-op (no restart)
	if m.thumbFadeTick != 5 {
		t.Errorf("re-affirming follow=true must not restart an in-flight fade, got tick=%d", m.thumbFadeTick)
	}
	m.setFollow(false)
	if m.thumbFading {
		t.Error("scrolling away again must cancel the fade outright")
	}
}

// The thumb paints ONLY in the gutter column, ONLY across the viewport's
// own rows, ONLY while scrolled away — and needs real margin room to show
// at all.
func TestScrollThumbGutterOnlyPlacementAndAbsentAtBottom(t *testing.T) {
	build := func(t *testing.T, width int) Model {
		t.Helper()
		m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true))
		m = drive(m, tea.WindowSizeMsg{Width: width, Height: 24})
		for i := int64(1); i <= 80; i++ {
			m.transcript.Append(api.Event{ID: i, TurnID: i, Kind: "system", Payload: []byte(`{"text":"row"}`)})
		}
		m.refreshViewport()
		return m
	}

	// The thumb rides the terminal's last column in every width mode
	// (owner ruling 2026-07-12).
	m := build(t, 108)
	m.sc.GotoBottom()
	m.follow = true
	if strings.ContainsAny(ansi.Strip(m.viewContent()), "⣿⡀") {
		t.Fatal("no thumb glyph while resting at the bottom")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	m = next.(Model)
	if m.follow {
		t.Fatal("setup: scrolling up must release follow")
	}

	view := ansi.Strip(m.viewContent())
	rows := strings.Split(view, "\n")
	vpHeight := m.chatViewportHeight()
	contentWidth := m.width - 1 // the right-edge scrollbar column
	found := 0
	for i := 0; i < vpHeight && i < len(rows); i++ {
		r := []rune(rows[i])
		if len(r) > contentWidth {
			switch r[contentWidth] {
			case '⣿', '⡀':
				found++
			}
		}
	}
	if found == 0 {
		t.Fatalf("expected the thumb/track glyph in the gutter column across the viewport rows:\n%s", view)
	}
	for i := vpHeight; i < len(rows); i++ {
		if strings.ContainsAny(rows[i], "⣿⡀") {
			t.Errorf("thumb glyphs must never reach past the viewport (row %d): %q", i, rows[i])
		}
	}

	// The right-edge thumb shows on any terminal ≥60 cols (owner loves
	// it); below that hard floor it stays away.
	narrow := build(t, 82)
	narrow.sc.GotoTop()
	narrow.follow = false
	if !strings.ContainsAny(ansi.Strip(narrow.viewContent()), "⣿⡀") {
		t.Error("an 82-col terminal shows the right-edge thumb")
	}
	tiny := build(t, 58)
	tiny.sc.GotoTop()
	tiny.follow = false
	if strings.ContainsAny(ansi.Strip(tiny.viewContent()), "⣿⡀") {
		t.Error("below the 60-col floor the thumb must stay away")
	}
}

// Returning to the bottom fades the thumb out over its own budget rather
// than snapping it away, and the gate closes once the fade settles.
func TestScrollThumbFadesOutAfterReturningToBottom(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true))
	m = drive(m, tea.WindowSizeMsg{Width: 108, Height: 24})
	for i := int64(1); i <= 80; i++ {
		m.transcript.Append(api.Event{ID: i, TurnID: i, Kind: "system", Payload: []byte(`{"text":"row"}`)})
	}
	m.refreshViewport()
	m.sc.GotoTop()
	m.follow = false

	for i := 0; i < 50 && !m.follow; i++ {
		m = drive(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	}
	if !m.follow {
		t.Fatal("setup: ctrl+d to the bottom must restore follow")
	}
	if !m.thumbFading {
		t.Fatal("returning to the bottom must start the thumb's fade-out")
	}
	if !m.animGateOpen() {
		t.Fatal("an in-flight fade must hold the gate open")
	}

	for i := int64(0); i < thumbFadeTicks && m.thumbFading; i++ {
		m = drive(m, AnimTickMsg{})
	}
	if m.thumbFading {
		t.Error("the fade must settle within its own ~800ms budget")
	}
	if m.animGateOpen() {
		t.Error("the gate must close once the fade settles (nothing else pending)")
	}
}
