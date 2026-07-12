package ui

import (
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
)

// ── Effect 1: startup splash ─────────────────────────────────────────────

func TestSplashLogoIsPitosExactBlocks(t *testing.T) {
	// The logo is pito's own LOGO_LINES, glyph for glyph (start_screen/
	// component.rb) — six rows, equal rune width, only blocks/box
	// connectors/spaces.
	if len(pitoLogoLines) != 6 {
		t.Fatalf("got %d rows, want 6", len(pitoLogoLines))
	}
	want := len([]rune(pitoLogoLines[0]))
	for i, row := range pitoLogoLines {
		if got := len([]rune(row)); got != want {
			t.Errorf("row %d width %d, want %d", i, got, want)
		}
		for _, ru := range row {
			switch ru {
			case '█', '╗', '║', '╔', '═', '╝', '╚', ' ':
			default:
				t.Errorf("row %d carries a foreign glyph %q", i, ru)
			}
		}
	}
	if pitoLogoLines[0] != "██████╗ ██╗████████╗ ██████╗ " {
		t.Errorf("row 0 drifted from pito's art: %q", pitoLogoLines[0])
	}
}

func TestSplashOffByDefaultInTestHarness(t *testing.T) {
	// newTestModel always prepends WithSplash(false) — the harness default
	// every OTHER test in this package relies on for a clean first frame.
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true))
	m = sized(m)
	if m.splashActive() {
		t.Fatal("splash should stay off unless a test explicitly re-arms it with WithSplash(true)")
	}
}

func TestSplashRequiresTruecolor(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithSplash(true))
	m = sized(m)
	if m.splashActive() {
		t.Fatal("splash must stay off on a non-truecolor terminal even when WithSplash(true)")
	}
}

func TestSplashArmsOnFirstReadyAndSkipsOnAnyKey(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true), WithSplash(true))
	m = sized(m)
	if !m.splashActive() {
		t.Fatal("splash should be active immediately after the first resize")
	}
	if !m.animating {
		t.Fatal("the fast tick loop should already be running for the splash")
	}
	m = drive(m, key("x"))
	if m.splashActive() {
		t.Fatal("any key should skip the splash instantly")
	}
}

func TestSplashDoesNotReArmOnLaterResize(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true), WithSplash(true))
	m = sized(m)
	m.skipSplash() // simulate the splash having already finished/been skipped
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 30})
	if m.splashActive() {
		t.Fatal("a later resize must never re-arm the splash")
	}
}

func TestSplashSettlesOnItsOwnWithoutAKeypress(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"), WithTruecolor(true), WithSplash(true))
	m = sized(m)
	if !m.splashActive() {
		t.Fatal("splash should be active after the first resize")
	}
	m = driveAnim(t, m, int(splashHoldTicks)+200)
	if m.splashActive() {
		t.Fatal("splash should have settled (hold + rise-away) within the tick budget")
	}
}

// ── Effect 2: keymap footer ──────────────────────────────────────────────

func TestKeymapFooterNonTruecolorMatchesOldHelpLine(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithNewConversation())
	m = sized(m)
	if got := m.keymapFooterView(); got != "" {
		t.Fatalf("footer should be empty before '?' is pressed, got %q", got)
	}
	m = drive(m, key("?"))
	want := m.helpLine()
	if got := m.keymapFooterView(); got != want {
		t.Fatalf("off-truecolor footer = %q, want byte-identical to helpLine() = %q", got, want)
	}
}

func TestKeymapFooterListsEveryBinding(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithNewConversation(), WithTruecolor(true))
	m = sized(m)
	m = drive(m, key("?"))
	m = driveAnim(t, m, 200)
	got := m.keymapFooterView()
	if got == "" {
		t.Fatal("footer should be open (non-empty) after '?' and settling")
	}
	for _, b := range footerKeymapDefault.bindings() {
		label := b.Help().Key
		if !strings.Contains(got, label) {
			t.Errorf("footer missing binding %q: %s", label, got)
		}
	}
	for _, g := range footerKeymapDefault.groups() {
		if !strings.Contains(got, g.label) {
			t.Errorf("footer missing group label %q: %s", g.label, got)
		}
	}
}

func TestKeymapFooterClosesBackToEmpty(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithNewConversation(), WithTruecolor(true))
	m = sized(m)
	m = drive(m, key("?"))
	m = driveAnim(t, m, 200)
	if m.keymapFooterView() == "" {
		t.Fatal("footer should be open after '?' and settling")
	}
	m = drive(m, key("?"))
	m = driveAnim(t, m, 200)
	if got := m.keymapFooterView(); got != "" {
		t.Fatalf("footer should be fully closed after toggling '?' again and settling, got %q", got)
	}
}

// ── Effect 3: OSC window title ───────────────────────────────────────────

func TestWindowTitleText(t *testing.T) {
	cases := []struct {
		label  string
		unread int
		want   string
	}{
		{"new conversation", 0, "pito · new conversation"},
		{"release prep", 0, "pito · release prep"},
		{"release prep", 3, "pito · release prep · ✉ 3"},
		{"(unnamed)", 1, "pito · (unnamed) · ✉ 1"},
	}
	for _, c := range cases {
		if got := windowTitleText(c.label, c.unread); got != c.want {
			t.Errorf("windowTitleText(%q, %d) = %q, want %q", c.label, c.unread, got, c.want)
		}
	}
}

func TestModelWindowTitleLabelFallbacks(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler())

	m.conv = api.Conversation{}
	if got, want := m.windowTitle(), "pito · new conversation"; got != want {
		t.Errorf("blank uuid: windowTitle() = %q, want %q", got, want)
	}

	m.conv = api.Conversation{UUID: "u1"}
	if got, want := m.windowTitle(), "pito · (unnamed)"; got != want {
		t.Errorf("uuid, no label: windowTitle() = %q, want %q", got, want)
	}

	m.conv = api.Conversation{UUID: "u1", DisplayName: "release prep"}
	m.unread = 2
	if got, want := m.windowTitle(), "pito · release prep · ✉ 2"; got != want {
		t.Errorf("named + unread: windowTitle() = %q, want %q", got, want)
	}
}

// ── Effect 4: OSC 8 hyperlinks ───────────────────────────────────────────

func TestWrapPlainURLsWrapsURLLeavesRestUntouched(t *testing.T) {
	text := "share it: https://dev.pitomd.com/share/abc123 enjoy"
	got := wrapPlainURLs(text)

	if !strings.HasPrefix(got, "share it: ") {
		t.Errorf("leading text changed: %q", got)
	}
	if !strings.HasSuffix(got, " enjoy") {
		t.Errorf("trailing text changed: %q", got)
	}
	if !strings.Contains(got, "\x1b]8;;https://dev.pitomd.com/share/abc123\a") {
		t.Errorf("missing OSC 8 open sequence: %q", got)
	}
	if !strings.Contains(got, "\x1b]8;;\a") {
		t.Errorf("missing OSC 8 reset sequence: %q", got)
	}
	if !strings.Contains(got, "https://dev.pitomd.com/share/abc123") {
		t.Errorf("visible URL text lost: %q", got)
	}
}

func TestWrapPlainURLsNoURLIsUntouched(t *testing.T) {
	text := "no links here, just prose"
	if got := wrapPlainURLs(text); got != text {
		t.Errorf("wrapPlainURLs(%q) = %q, want unchanged", text, got)
	}
}

func TestWrapPlainURLsTrimsTrailingPunctuation(t *testing.T) {
	got := wrapPlainURLs("See https://x.io/a. Enjoy!")
	resetIdx := strings.Index(got, "\x1b]8;;\a")
	if resetIdx < 0 {
		t.Fatalf("missing OSC 8 reset sequence: %q", got)
	}
	after := got[resetIdx+len("\x1b]8;;\a"):]
	if !strings.HasPrefix(after, ". Enjoy!") {
		t.Errorf("trailing period should sit OUTSIDE the hyperlink, got tail %q (full: %q)", after, got)
	}
	if strings.Contains(got, "a.\a") {
		t.Errorf("trailing period leaked inside the hyperlink target: %q", got)
	}
}

func TestOSC8EligibleKind(t *testing.T) {
	cases := []struct {
		kind string
		want bool
	}{
		{api.KindSystem, true},
		{api.KindSystemFollowUp, true},
		{api.KindEnhanced, false},
		{api.KindEnhancedFollowUp, false},
		{api.KindAi, false},
		{api.KindError, false},
		{api.KindThinking, false},
		{api.KindConfirmation, false},
		{"unknown", false},
	}
	for _, c := range cases {
		if got := osc8EligibleKind(c.kind); got != c.want {
			t.Errorf("osc8EligibleKind(%q) = %v, want %v", c.kind, got, c.want)
		}
	}
}

func TestOSC8SafeFromStructure(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    bool
	}{
		{"plain text", `{"text":"hi https://x.io"}`, true},
		{"table rows", `{"table_rows":[{"cells":[{"text":"x"}]}]}`, false},
		{"sections", `{"sections":[{"title":"t"}]}`, false},
		{"games", `{"games":[{"id":1,"title":"g","vids":2}]}`, false},
		{"analyze", `{"analyze":{"intro":"crunching"}}`, false},
		{"malformed json degrades safe", `not json`, true},
	}
	for _, c := range cases {
		if got := osc8SafeFromStructure([]byte(c.payload)); got != c.want {
			t.Errorf("%s: osc8SafeFromStructure = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWrapEventLinksGating(t *testing.T) {
	block := "visit https://x.io/y"

	systemEv := api.Event{Kind: api.KindSystem, Payload: []byte(`{"text":"visit https://x.io/y"}`)}
	if got := wrapEventLinks(systemEv, block); !strings.Contains(got, "\x1b]8;;https://x.io/y\a") {
		t.Errorf("eligible system event should be wrapped, got %q", got)
	}

	aiEv := api.Event{Kind: api.KindAi, Payload: []byte(`{}`)}
	if got := wrapEventLinks(aiEv, block); got != block {
		t.Errorf("ai-kind event must never be wrapped, got %q", got)
	}

	tableEv := api.Event{Kind: api.KindSystem, Payload: []byte(`{"table_rows":[{"cells":[{"text":"x"}]}]}`)}
	if got := wrapEventLinks(tableEv, block); got != block {
		t.Errorf("system event carrying table_rows must never be wrapped, got %q", got)
	}
}

// TestOnResizeWrapsShareURLEndToEnd drives the REAL pipeline (onResize's
// SetRenderer seam) rather than calling wrapEventLinks directly — a system
// event with a share URL lands on the cable, and the rendered transcript
// should carry the OSC 8 escape around it.
func TestOnResizeWrapsShareURLEndToEnd(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	m = drive(m, CableEventMsg{M: cable.StreamMessage{
		Type: cable.TypeEventAppend,
		Event: api.Event{
			ID: 1, TurnID: 1, Kind: api.KindSystem,
			Payload: []byte(`{"text":"Shared! https://dev.pitomd.com/share/abc123 — enjoy."}`),
		},
	}})
	view := m.transcript.View(m.contentWidth())
	if !strings.Contains(view, "\x1b]8;;https://dev.pitomd.com/share/abc123\a") {
		t.Fatalf("rendered transcript missing OSC 8 hyperlink wrap: %q", view)
	}
}
