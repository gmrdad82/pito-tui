package render

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/ansi"
)

// ── text ─────────────────────────────────────────────────────────────────

func TestAiTextBlockInlineMarkup(t *testing.T) {
	payload := `{"type":"text","text":"**bold** and *italic* and [cyan]cyan word[/cyan] and [red]bad[/red] and [green]good[/green] and [subject]Elden Ring[/subject] and [ref]#12[/ref]."}`
	out := stripANSI(plain().aiTextBlock(json.RawMessage(payload), 200))
	for _, want := range []string{"bold", "italic", "cyan word", "bad", "good", "Elden Ring", "#12"} {
		if !strings.Contains(out, want) {
			t.Errorf("text block lost %q:\n%s", want, out)
		}
	}
	for _, notation := range []string{"**", "[cyan]", "[/cyan]", "[red]", "[/red]", "[green]", "[/green]", "[subject]", "[/subject]", "[ref]", "[/ref]"} {
		if strings.Contains(out, notation) {
			t.Errorf("inline notation %q leaked into output:\n%s", notation, out)
		}
	}
}

func TestAiTextBlockNewlinesPreserved(t *testing.T) {
	payload := `{"type":"text","text":"first paragraph\n\nsecond paragraph\nthird line"}`
	out := plain().aiTextBlock(json.RawMessage(payload), 200)
	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines (para/blank/para/line), got %d:\n%q", len(lines), out)
	}
	// A blank line marks the paragraph break.
	blank := false
	for _, l := range lines {
		if l == "" {
			blank = true
		}
	}
	if !blank {
		t.Errorf("paragraph break did not survive as a blank line:\n%q", out)
	}
	if !strings.Contains(out, "first paragraph") || !strings.Contains(out, "second paragraph") || !strings.Contains(out, "third line") {
		t.Errorf("newline-separated content lost:\n%q", out)
	}
}

func TestAiTextBlockWordWraps(t *testing.T) {
	payload := `{"type":"text","text":"one two three four five six seven eight nine ten"}`
	out := plain().aiTextBlock(json.RawMessage(payload), 15)
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 15 {
			t.Errorf("line exceeds width 15 (%d): %q", w, line)
		}
	}
	if !strings.Contains(out, "one") || !strings.Contains(out, "ten") {
		t.Errorf("wrapped content lost words:\n%s", out)
	}
}

func TestAiTextBlockEscaping(t *testing.T) {
	// Raw HTML, markdown-adjacent syntax, and a raw ANSI escape sequence
	// must never be interpreted — text blocks never run through an HTML
	// parser or glamour, and control bytes are stripped outright.
	escByte := string(rune(0x1b))
	text := "<b>not html</b> # not a heading `not code` " + escByte + "[31mFAKE" + escByte + "[0m"
	raw, err := json.Marshal(struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{"text", text})
	if err != nil {
		t.Fatal(err)
	}
	out := plain().aiTextBlock(raw, 200)
	for _, want := range []string{"<b>not html</b>", "# not a heading", "`not code`", "FAKE"} {
		if !strings.Contains(out, want) {
			t.Errorf("literal text lost during escaping %q:\n%q", want, out)
		}
	}
	if strings.ContainsRune(out, rune(0x1b)) {
		t.Errorf("raw ANSI escape byte survived sanitization:\n%q", out)
	}
}

func TestAiTextBlockMalformedAndEmpty(t *testing.T) {
	if out := plain().aiTextBlock(json.RawMessage(`not json`), 60); out != "" {
		t.Errorf("malformed JSON must render \"\": %q", out)
	}
	if out := plain().aiTextBlock(json.RawMessage(`{"type":"text","text":""}`), 60); out != "" {
		t.Errorf("blank text must render \"\": %q", out)
	}
	if out := plain().aiTextBlock(json.RawMessage(`{"type":"text","text":"   "}`), 60); out != "" {
		t.Errorf("whitespace-only text must render \"\": %q", out)
	}
}

// ── kv_table ─────────────────────────────────────────────────────────────

func TestAiKvTableBlockPlainValues(t *testing.T) {
	payload := `{"type":"kv_table","rows":[["Genre","Action RPG"],["Developer","FromSoftware"]]}`
	out := plain().aiKvTableBlock(json.RawMessage(payload), 60)
	for _, want := range []string{"Genre", "Action RPG", "Developer", "FromSoftware"} {
		if !strings.Contains(out, want) {
			t.Errorf("kv_table lost %q:\n%s", want, out)
		}
	}
}

// TestAiKvTableBlockPlainValueAligns pins the extended right-alignment
// census (owner decree — numeric + #id + date/time) applied to kv_table
// PLAIN string values: a plain value matching any of the three shape
// families right-aligns within the value column exactly the way a typed
// value already does (formatAiKvValue's rightAlign path — see
// TestAiKvTableBlockPriceFormat et al for the typed side, unaffected by
// this change); prose left-aligns; keys are never tested for alignment.
func TestAiKvTableBlockPlainValueAligns(t *testing.T) {
	// A single unbroken "word" as the widest value: wrapPlain never splits
	// a lone word regardless of width (detail.go), so this cell is
	// guaranteed to render on one line and safely anchor naturalVal,
	// decoupling the padding math below from any wrapping decision.
	longValue := strings.Repeat("z", 40)
	payload := `{"type":"kv_table","rows":[
		["ID","#38"],
		["Views","7,709"],
		["Aired","19 Jul 12:00"],
		["Note","a short prose value"],
		["Score",{"v":"94","format":"score"}],
		["Padding","` + longValue + `"]
	]}`
	width := 100
	out := stripANSI(plain().aiKvTableBlock(json.RawMessage(payload), width))
	lines := strings.Split(out, "\n")

	naturalKey := lipgloss.Width("Padding") // the widest key among the rows above
	naturalVal := lipgloss.Width(longValue) // the widest single-line value
	keyWidth := aiKvKeyWidth(naturalKey, naturalVal, width)
	valWidth := width - keyWidth - 3

	wantLine := func(key, value string, rightAlign bool) string {
		pad := max(keyWidth-lipgloss.Width(key), 0)
		keyCell := " " + key + strings.Repeat(" ", pad) + "  "
		if !rightAlign {
			return keyCell + value
		}
		valPad := valWidth - lipgloss.Width(value)
		if valPad <= 0 {
			t.Fatalf("test setup: %q leaves no visible left pad to observe (valWidth=%d)", value, valWidth)
		}
		return keyCell + strings.Repeat(" ", valPad) + value
	}
	assertLine := func(want string) {
		t.Helper()
		for _, l := range lines {
			if l == want {
				return
			}
		}
		t.Errorf("expected line %q not found in:\n%s", want, out)
	}

	assertLine(wantLine("ID", "#38", true))
	assertLine(wantLine("Views", "7,709", true))
	assertLine(wantLine("Aired", "19 Jul 12:00", true))
	assertLine(wantLine("Score", "94", true))
	assertLine(wantLine("Note", "a short prose value", false))
}

func TestAiKvTableBlockPriceFormat(t *testing.T) {
	cases := []struct {
		name  string
		v     string
		coins int
		want  []string
		none  []string
	}{
		{"budget", "9.99", 1, []string{"9.99"}, nil},
		{"mid", "59.99", 3, []string{"59.99"}, nil},
		{"premium", "99.00", 5, []string{"99.00"}, nil},
		{"free", "0", 0, []string{"0.00", "★"}, []string{"●"}},
		{"negative", "-5", 0, []string{"—"}, []string{"●", "★"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			payload := `{"type":"kv_table","rows":[["Price",{"v":"` + c.v + `","format":"price"}]]}`
			out := plain().aiKvTableBlock(json.RawMessage(payload), 60)
			if !strings.Contains(out, "Price") {
				t.Errorf("key lost:\n%s", out)
			}
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("price render missing %q:\n%s", want, out)
				}
			}
			for _, absent := range c.none {
				if strings.Contains(out, absent) {
					t.Errorf("price render must not contain %q:\n%s", absent, out)
				}
			}
			if c.coins > 0 {
				if n := strings.Count(out, "●"); n != c.coins {
					t.Errorf("%s: want %d coin glyphs, got %d:\n%s", c.name, c.coins, n, out)
				}
			}
		})
	}
}

func TestAiKvTableBlockDateFormat(t *testing.T) {
	// House shape (owner decree 2026-07-19, generalizing
	// TimestampPrefixComponent to every date on every surface) — unit
	// coverage of every shape/edge case lives in TestFormatAiDate below;
	// this is just the end-to-end kv_table wiring check, clock pinned so
	// the current-year assertion never drifts with the calendar.
	fixedNow := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	r := New(60, WithPlain(), WithNow(func() time.Time { return fixedNow }))
	payload := `{"type":"kv_table","rows":[["Released",{"v":"2026-07-10","format":"date"}],["Unparseable",{"v":"not-a-date","format":"date"}]]}`
	out := r.aiKvTableBlock(json.RawMessage(payload), 60)
	if !strings.Contains(out, "10 Jul") {
		t.Errorf("date must render \"10 Jul\":\n%s", out)
	}
	if !strings.Contains(out, "not-a-date") {
		t.Errorf("unparseable date must fall back to the raw string:\n%s", out)
	}
}

// TestFormatAiDate table-drives formatAiDate's four strict shapes plus its
// fallback rules against the house date format (owner decree 2026-07-19,
// generalizing TimestampPrefixComponent — timestamp_prefix_component.rb:
// 38-47 — to every date on every surface): date-only never collapses on
// "today", a datetime does. now is injected as a fixed clock (never
// time.Now) so every current-year/today assertion is deterministic — the
// zoned-instant cases still convert through time.Local (TestMain pins it to
// UTC for this package) exactly like the live path, but the today/current-
// year DECISION is made against the fixed now below, not the real clock.
func TestFormatAiDate(t *testing.T) {
	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		in   string
		want string
	}{
		// date-only, current year — never collapses even when the date
		// literally IS now's date (dropping it would leave nothing).
		{"iso date, current year", "2026-07-10", "10 Jul"},
		{"dmy date, current year", "10-07-2026", "10 Jul"},
		{"iso date, current year, IS today, date still shows", "2026-07-19", "19 Jul"},
		// date-only, other year — past and future both take the 'YY suffix.
		{"iso date, other year (past)", "2025-12-31", "31 Dec '25"},
		{"dmy date, other year (future)", "05-03-2027", "5 Mar '27"},
		// datetime, today — collapses to a bare clock, date drops entirely.
		{"dmy datetime, today, collapses to bare HH:MM", "19-07-2026 08:15", "08:15"},
		{"iso datetime, today, seconds omitted, collapses", "2026-07-19T23:59", "23:59"},
		// datetime, current year, not today — day+month, no year.
		{"iso datetime, current year, zoneless keeps its wall clock", "2026-07-10T14:30:00", "10 Jul 14:30"},
		{"dmy datetime, current year, not today", "10-07-2026 14:30", "10 Jul 14:30"},
		{"iso datetime, zoned (Z), current year not today", "2026-07-05T09:00:00Z", "5 Jul 09:00"},
		// datetime, other year — day+month+'YY.
		{"dmy datetime, other year", "10-07-2025 14:30", "10 Jul '25 14:30"},
		{"iso datetime, other year, zoneless", "2027-01-02T09:00:00", "2 Jan '27 09:00"},
		// A zoned instant's today/current-year decision runs AFTER the
		// local conversion, not on the wire's literal date substring —
		// both directions of the boundary crossing, verified explicitly.
		{"iso datetime, zoned, literal date is yesterday but converts INTO today", "2026-07-18T23:30:00-02:00", "01:30"},
		{"iso datetime, zoned, literal date is today but converts OUT of today", "2026-07-19T23:30:00-05:00", "20 Jul 04:30"},
		// Parse failures and non-typed shapes: raw passthrough, unaffected
		// by now.
		{"invalid calendar date (iso, day out of range)", "2026-02-30", "2026-02-30"},
		{"invalid calendar date (dmy, month out of range)", "13-13-2026", "13-13-2026"},
		{"old-lenient shape: US month/day/year", "01/02/2026", "01/02/2026"},
		{"old-lenient shape: US \"Mon D, YYYY\"", "Jan 2, 2026", "Jan 2, 2026"},
		{"garbage", "not-a-date", "not-a-date"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatAiDate(c.in, now); got != c.want {
				t.Errorf("formatAiDate(%q, %v) = %q, want %q", c.in, now, got, c.want)
			}
		})
	}
}

func TestAiKvTableBlockNumberFormat(t *testing.T) {
	payload := `{"type":"kv_table","rows":[
		["Views",{"v":"2300","format":"number"}],
		["Subs",{"v":"15000000","format":"number"}],
		["Delta",{"v":"-5000000","format":"number"}],
		["Junk",{"v":"abc","format":"number"}]
	]}`
	out := plain().aiKvTableBlock(json.RawMessage(payload), 60)
	for _, want := range []string{"2.3K", "15M", "-5000000", "0"} {
		if !strings.Contains(out, want) {
			t.Errorf("compact number missing %q:\n%s", want, out)
		}
	}
}

func TestAiKvTableBlockScoreFormat(t *testing.T) {
	// kv_table score is a bare integer, NOT clamped to 0..100 (unlike the
	// standalone score/heart blocks) and carries no "/100" suffix.
	payload := `{"type":"kv_table","rows":[["Metacritic",{"v":"94","format":"score"}],["OutOfRange",{"v":"150","format":"score"}]]}`
	out := plain().aiKvTableBlock(json.RawMessage(payload), 60)
	if !strings.Contains(out, "94") || !strings.Contains(out, "150") {
		t.Errorf("score values lost:\n%s", out)
	}
	if strings.Contains(out, "/100") {
		t.Errorf("kv_table score must not carry a /100 suffix:\n%s", out)
	}
}

func TestAiKvTableBlockSkipsBadRowsKeepsGoodOnes(t *testing.T) {
	payload := `{"type":"kv_table","rows":[
		{"key":"objectShaped","value":"unsupported row shape"},
		["","blank key dropped"],
		["Good","survives"]
	]}`
	out := plain().aiKvTableBlock(json.RawMessage(payload), 60)
	if !strings.Contains(out, "Good") || !strings.Contains(out, "survives") {
		t.Errorf("valid row dropped alongside bad ones:\n%s", out)
	}
	if strings.Contains(out, "blank key dropped") {
		t.Errorf("a blank-key row must not render:\n%s", out)
	}
}

// TestAiKvTableBlockAlignsToCharmStyling pins the Charm restyle (owner
// 2026-07-12 "align to Charm") on the @ai kv-table block: cyan keys stay
// untouched (house rule, meaning not decoration), values alternate
// ColorFaint(241, even rows)/ColorDim(245, odd rows), no background SGR
// survives — the old plum stripe is gone.
func TestAiKvTableBlockAlignsToCharmStyling(t *testing.T) {
	withTrueColor(t)
	payload := `{"type":"kv_table","rows":[["Title","Alpha"],["Genre","Bravo"],["Dev","Charlie"]]}`
	out := New(80, WithTruecolor(true)).aiKvTableBlock(json.RawMessage(payload), 60)
	if strings.Contains(out, "48;2;") || strings.Contains(out, "48;5;") {
		t.Errorf("kv table block must carry no background SGR at all:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;241mAlpha\x1b[m") {
		t.Errorf("even value row must use ColorFaint (241):\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;245mBravo\x1b[m") {
		t.Errorf("odd value row must use ColorDim (245):\n%q", out)
	}
	if strings.Count(out, "\x1b[38;5;44m") != 3 {
		t.Errorf("every key must keep the house cyan (ColorCyan 44):\n%q", out)
	}
}

func TestAiKvTableBlockEmptyOrMalformed(t *testing.T) {
	if out := plain().aiKvTableBlock(json.RawMessage(`not json`), 60); out != "" {
		t.Errorf("malformed JSON must render \"\": %q", out)
	}
	if out := plain().aiKvTableBlock(json.RawMessage(`{"type":"kv_table","rows":[]}`), 60); out != "" {
		t.Errorf("no rows must render \"\": %q", out)
	}
}

// TestAiKvKeyWidth table-drives aiKvKeyWidth directly — the pure,
// deterministic width-aware KEY column allocator (owner decree
// 2026-07-19) — pinning exact allocated widths apart from any row
// rendering: the desktop fit case, the exact boundary of the fit check,
// the narrow-terminal floor, the recompute branch that gives the value
// column back its 10-cell floor, and the degenerate case where the key is
// already narrower than the floor (capped at its own natural width, never
// padded past it).
func TestAiKvKeyWidth(t *testing.T) {
	cases := []struct {
		name                          string
		naturalKey, naturalVal, width int
		want                          int
	}{
		{"wide desktop terminal: key renders in full, no shrink at all", 30, 1, 80, 30},
		{"exact boundary of the fit check: still fits at ==, not just <", 10, 5, 23, 10},
		{"one cell under the fit boundary: still capped at its own natural width", 10, 5, 22, 10},
		{"narrow terminal: a 30-cell key floors at aiKvKeyMaxWidth", 30, 1, 22, 20},
		{"big key + small value: the value's 10-cell floor pulls the key in further", 55, 2, 60, 47},
		{"short key + oversize value: degenerate, key stays at its own natural width", 15, 20, 20, 15},
		{"the boundary where the value floor exactly engages wrapping", 25, 40, 33, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := aiKvKeyWidth(c.naturalKey, c.naturalVal, c.width); got != c.want {
				t.Errorf("aiKvKeyWidth(%d, %d, %d) = %d, want %d", c.naturalKey, c.naturalVal, c.width, got, c.want)
			}
		})
	}
}

// TestAiKvTableBlockKeyTruncatesUnderWidthPressure pins the NARROW side of
// the width-aware KEY column rule (owner decree 2026-07-19, aiKvKeyWidth):
// aiKvKeyMaxWidth is a PRESSURE FLOOR now, not the old unconditional cap —
// a key only shrinks toward it when the row genuinely doesn't have the
// room (see TestAiKvTableBlockWideTerminalLongKeyNeverTruncates for the
// desktop case this test used to get wrong at width 60, back when the cap
// was unconditional). At a narrow width, a 30-cell key still floors at 20
// and truncates with an ellipsis, and the value column ends up exactly as
// wide as it would for a key that was already 20 cells — the starvation
// the floor exists to prevent never happens.
func TestAiKvTableBlockKeyTruncatesUnderWidthPressure(t *testing.T) {
	const narrow = 22                  // too tight for a 30-cell key to render in full
	longKey := strings.Repeat("A", 30) // 30 cells, well past the 20 floor
	wantKey := ansi.Truncate(longKey, aiKvKeyMaxWidth, "…")
	if w := lipgloss.Width(wantKey); w != aiKvKeyMaxWidth || !strings.HasSuffix(wantKey, "…") {
		t.Fatalf("test setup: ansi.Truncate(longKey, %d, \"…\") = %q (width %d), want width %d ending in an ellipsis", aiKvKeyMaxWidth, wantKey, w, aiKvKeyMaxWidth)
	}

	payload := `{"type":"kv_table","rows":[["` + longKey + `",{"v":"5","format":"score"}]]}`
	out := plain().aiKvTableBlock(json.RawMessage(payload), narrow)
	if !strings.Contains(out, wantKey) {
		t.Errorf("a key over the floor under width pressure must truncate to %q:\n%q", wantKey, out)
	}
	if strings.Contains(out, longKey) {
		t.Errorf("the untruncated key must not survive under width pressure:\n%q", out)
	}

	// Same row, but the key is exactly the floor width already — the value
	// column (padding included) must render identically either way, since
	// keyWidth floors to the same 20 cells in both cases.
	cappedKey := strings.Repeat("B", aiKvKeyMaxWidth)
	cappedPayload := `{"type":"kv_table","rows":[["` + cappedKey + `",{"v":"5","format":"score"}]]}`
	cappedOut := plain().aiKvTableBlock(json.RawMessage(cappedPayload), narrow)
	if lipgloss.Width(out) != lipgloss.Width(cappedOut) {
		t.Errorf("value column width must match what a %d-cell key gives: truncated-row width %d vs capped-row width %d", aiKvKeyMaxWidth, lipgloss.Width(out), lipgloss.Width(cappedOut))
	}
}

// TestAiKvTableBlockWideTerminalLongKeyNeverTruncates pins the owner-caught
// regression itself (2026-07-19): on a wide terminal — the desktop case —
// a key well past the old unconditional 20-cell cap renders in FULL, no
// ellipsis anywhere. This is the behavior yesterday's aiKvKeyMaxWidth cap
// broke; aiKvKeyWidth's fit check is the fix.
func TestAiKvTableBlockWideTerminalLongKeyNeverTruncates(t *testing.T) {
	longKey := strings.Repeat("A", 30) // 30 cells, well past the old cap
	payload := `{"type":"kv_table","rows":[["` + longKey + `",{"v":"5","format":"score"}]]}`
	out := plain().aiKvTableBlock(json.RawMessage(payload), 80)
	if !strings.Contains(out, longKey) {
		t.Errorf("a wide terminal must render a >20-cell key in full, untruncated:\n%q", out)
	}
	if strings.Contains(out, "…") {
		t.Errorf("no ellipsis should appear anywhere on a wide terminal:\n%q", out)
	}
}

// TestAiKvTableBlockValueFloorEngagesWrapping pins the boundary where the
// KEY column has already floored at aiKvKeyMaxWidth and the VALUE column
// sits right at its own 10-cell floor: a long plain value still can't fit
// on one line at that width, so it wraps across multiple lines via the
// existing wrapPlain path exactly as it always has — the width-aware key
// rule only changes when the floor engages, never what happens once it
// does.
func TestAiKvTableBlockValueFloorEngagesWrapping(t *testing.T) {
	longKey := strings.Repeat("K", 25) // over the floor: must shrink to 20
	value := "one two three four five six seven eight nine ten"
	payload := `{"type":"kv_table","rows":[["` + longKey + `","` + value + `"]]}`
	out := stripANSI(plain().aiKvTableBlock(json.RawMessage(payload), 33))
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected the value to wrap at the value floor, got 1 line:\n%q", out)
	}
	wantKey := ansi.Truncate(longKey, aiKvKeyMaxWidth, "…")
	if !strings.HasPrefix(lines[0], " "+wantKey) {
		t.Errorf("first line must lead with the key floored at %d cells:\n%q", aiKvKeyMaxWidth, lines[0])
	}
	wantIndent := strings.Repeat(" ", aiKvKeyMaxWidth+3)
	for i, line := range lines[1:] {
		if !strings.HasPrefix(line, wantIndent) {
			t.Errorf("continuation line %d must indent by the floored key width+3 (%d spaces):\n%q", i+1, aiKvKeyMaxWidth+3, line)
		}
	}
}

// TestAiKvTableBlockKeyAtOrUnderFloorUnchanged pins the floor's other
// side: a key at or under aiKvKeyMaxWidth cells never truncates, whether
// it takes the fit branch (comfortably under width) or the shrink branch
// (clamped, but already at or under its own natural width) — unchanged
// behavior either way.
func TestAiKvTableBlockKeyAtOrUnderFloorUnchanged(t *testing.T) {
	exactlyAtFloor := strings.Repeat("C", aiKvKeyMaxWidth)
	underFloor := "Genre"
	payload := `{"type":"kv_table","rows":[["` + exactlyAtFloor + `","full width"],["` + underFloor + `","short value"]]}`
	out := plain().aiKvTableBlock(json.RawMessage(payload), 60)
	if !strings.Contains(out, exactlyAtFloor) {
		t.Errorf("a key exactly at the floor must render unchanged:\n%q", out)
	}
	if !strings.Contains(out, underFloor) {
		t.Errorf("a key under the floor must render unchanged:\n%q", out)
	}
	if strings.Contains(out, "…") {
		t.Errorf("no key at/under the floor should ever truncate:\n%q", out)
	}
}

// TestAiKvTableBlockWrappedValueAlignsUnderTruncatedKey pins the
// continuation-line indent (keyWidth+3 spaces): when the key truncates,
// wrapped value lines must still indent by the FLOORED width, not the raw
// over-floor key's width, so every row keeps aligning on one column.
func TestAiKvTableBlockWrappedValueAlignsUnderTruncatedKey(t *testing.T) {
	longKey := strings.Repeat("Z", 30)
	value := "one two three four five six seven eight nine ten"
	payload := `{"type":"kv_table","rows":[["` + longKey + `","` + value + `"]]}`
	out := stripANSI(plain().aiKvTableBlock(json.RawMessage(payload), 40))
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected the long value to wrap across multiple lines, got 1:\n%q", out)
	}
	wantKey := ansi.Truncate(longKey, aiKvKeyMaxWidth, "…")
	if !strings.HasPrefix(lines[0], " "+wantKey) {
		t.Errorf("first line must lead with the truncated key:\n%q", lines[0])
	}
	wantIndent := strings.Repeat(" ", aiKvKeyMaxWidth+3)
	for i, line := range lines[1:] {
		if !strings.HasPrefix(line, wantIndent) {
			t.Errorf("continuation line %d must indent by the floored key width+3 (%d spaces):\n%q", i+1, aiKvKeyMaxWidth+3, line)
		}
	}
}

// ── table ────────────────────────────────────────────────────────────────

func TestAiTableBlockRenders(t *testing.T) {
	payload := `{"type":"table","header":["Game","Score"],"rows":[["Elden Ring","96"],["Hades II","91"]]}`
	out := plain().aiTableBlock(json.RawMessage(payload), 60)
	for _, want := range []string{"Game", "Score", "Elden Ring", "96", "Hades II", "91"} {
		if !strings.Contains(out, want) {
			t.Errorf("table lost %q:\n%s", want, out)
		}
	}
	if !strings.ContainsAny(out, "─╭╮╰╯") {
		t.Errorf("expected a bordered frame:\n%s", out)
	}
}

// TestAiTableBlockAlignsToCharmStyling pins the Charm restyle (owner
// 2026-07-12 "align to Charm") on the @ai table block: bold purple header
// + rules (the exact truecolor hex #7D56F4 = 125,86,244), alternating
// ColorFaint(241)/ColorDim(245) data rows, no background SGR — the old
// plum zebra stripe is gone.
func TestAiTableBlockAlignsToCharmStyling(t *testing.T) {
	withTrueColor(t)
	payload := `{"type":"table","header":["#","Game"],"rows":[["1","Alpha"],["2","Bravo"],["3","Charlie"]]}`
	out := New(80, WithTruecolor(true)).aiTableBlock(json.RawMessage(payload), 60)
	if strings.Contains(out, "48;2;") || strings.Contains(out, "48;5;") {
		t.Errorf("table block must carry no background SGR at all:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[1;38;2;125;86;244m#\x1b[m") {
		t.Errorf("header cell must be bold + the exact truecolor Charm purple:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;2;125;86;244m─") {
		t.Errorf("rules must ride the same Charm purple as the header:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;241m1\x1b[m") || !strings.Contains(out, "\x1b[38;5;241m3\x1b[m") {
		t.Errorf("even data rows must use ColorFaint (241):\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;245m2\x1b[m") {
		t.Errorf("odd data rows must use ColorDim (245):\n%q", out)
	}
}

func TestAiTableBlockOversizeDegradesToTruncation(t *testing.T) {
	payload := `{"type":"table","header":["Game","Genre"],"rows":[
		["Demon's Souls","Role-playing (RPG), Hack and slash/Beat 'em up, Adventure"],
		["Cyberpunk 2077","Shooter, Role-playing (RPG), Adventure"]
	]}`
	out := plain().aiTableBlock(json.RawMessage(payload), 30)
	if !strings.Contains(out, "…") {
		t.Errorf("oversize table must truncate with an ellipsis, not wrap:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 30 {
			t.Errorf("line exceeds width 30 (%d): %q", w, line)
		}
	}
}

func TestAiTableBlockMissingHeaderOrRows(t *testing.T) {
	if out := plain().aiTableBlock(json.RawMessage(`{"type":"table","header":[],"rows":[["a"]]}`), 60); out != "" {
		t.Errorf("no header must render \"\": %q", out)
	}
	if out := plain().aiTableBlock(json.RawMessage(`{"type":"table","header":["A"],"rows":[]}`), 60); out != "" {
		t.Errorf("no rows must render \"\": %q", out)
	}
	if out := plain().aiTableBlock(json.RawMessage(`not json`), 60); out != "" {
		t.Errorf("malformed JSON must render \"\": %q", out)
	}
}

// TestAiTableAlignedCols pins aiTableAlignedCols (renamed from
// aiTableNumericCols now that alignment covers three shape families), the
// Go port of table_block_component.rb NUMERIC_CELL/ID_CELL/DATE_CELL +
// #align's column census: a column counts as ALIGNED when it has at least
// one non-empty BODY cell and every non-empty body cell matches numeric
// (grouped ints, K/M/B magnitudes, percent), #id ("#38"), or date/time
// (house stamp, bare HH:MM, ISO, or DMY) — cells may mix families within
// one column. Ruby quirks carried over verbatim: a cell of bare commas/dots
// matches (NUMERIC_CELL needs no digit, just `[\d,.]+`), and a single prose
// cell anywhere in the column drags the whole column back to left-align —
// "an all-empty column left-aligns" is only observable here, not on
// rendered padding: a column with no visible content anywhere (header
// included) has nothing for Align(Right) to shift.
func TestAiTableAlignedCols(t *testing.T) {
	cases := []struct {
		name   string
		header []string
		rows   [][]string
		want   []bool
	}{
		{
			name:   "prose column is not aligned",
			header: []string{"Game"},
			rows:   [][]string{{"Elden Ring"}, {"Hades II"}},
			want:   []bool{false},
		},
		{
			name:   "wire shapes are numeric: grouped int, K-suffix, percent",
			header: []string{"Views"},
			rows:   [][]string{{"7,709"}, {"2.2K"}, {"93%"}},
			want:   []bool{true},
		},
		{
			name:   "empty body cells among numbers stay aligned",
			header: []string{"Count"},
			rows:   [][]string{{"150"}, {""}, {"7"}},
			want:   []bool{true},
		},
		{
			name:   "all-empty column is not aligned (cells.any? guard)",
			header: []string{"Extra"},
			rows:   [][]string{{""}, {""}},
			want:   []bool{false},
		},
		{
			name:   "one prose cell among numbers is not aligned",
			header: []string{"Score"},
			rows:   [][]string{{"96"}, {"N/A"}, {"91"}},
			want:   []bool{false},
		},
		{
			name:   "bare commas/dots quirk ported verbatim",
			header: []string{"Weird"},
			rows:   [][]string{{"..."}, {",,"}},
			want:   []bool{true},
		},
		{
			name:   "columns classified independently",
			header: []string{"Game", "Score", "Note"},
			rows: [][]string{
				{"Elden Ring", "96", "great"},
				{"Hades II", "91", "ok"},
			},
			want: []bool{false, true, false},
		},
		{
			name:   "all-#id column is aligned",
			header: []string{"#"},
			rows:   [][]string{{"#38"}, {"#7"}, {"#1024"}},
			want:   []bool{true},
		},
		{
			name:   "\"TEKKEN #38\" does not count as an id cell",
			header: []string{"#"},
			rows:   [][]string{{"#38"}, {"TEKKEN #38"}},
			want:   []bool{false},
		},
		{
			name:   "date column mixes the house shape and a frozen DMY payload and is aligned",
			header: []string{"Released"},
			rows:   [][]string{{"19 Jul 12:00"}, {"5 Jun '25 12:00"}, {"19-07-2026 12:00"}, {"2026-07-19"}},
			want:   []bool{true},
		},
		{
			name:   "bare date-only house shapes are aligned",
			header: []string{"Released"},
			rows:   [][]string{{"2 Jan"}, {"31 Dec '25"}},
			want:   []bool{true},
		},
		{
			name:   "bare clock time is aligned",
			header: []string{"Time"},
			rows:   [][]string{{"08:15"}, {"23:59"}},
			want:   []bool{true},
		},
		{
			name:   "a column may mix families across rows (id + numeric)",
			header: []string{"Ref"},
			rows:   [][]string{{"#38"}, {"7,709"}},
			want:   []bool{true},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := aiTableAlignedCols(c.header, c.rows)
			if len(got) != len(c.want) {
				t.Fatalf("got %d columns, want %d", len(got), len(c.want))
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("column %d: got aligned=%v, want %v", i, got[i], c.want[i])
				}
			}
		})
	}
}

// tableCellPadding returns the leading/trailing space counts around the
// visible content of a single line from a stripANSI'd, single-column
// aiTableBlock render. lipgloss/table's Align(Right) fills unused column
// width BEFORE the cell's text; the left-aligned default fills AFTER it —
// so a cell strictly shorter than its column's widest cell exposes the
// direction as an asymmetry between lead and trail.
func tableCellPadding(t *testing.T, line string) (lead, trail int) {
	t.Helper()
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		t.Fatalf("no visible content on line %q", line)
	}
	lead = strings.Index(line, trimmed)
	trail = len(line) - lead - len(trimmed)
	return lead, trail
}

// TestAiTableBlockAlignedColumnRightAligns pins aligned-column alignment on
// actual rendered output (table_block_component.rb#align — "pito's own
// table law", web parity), across all three shape families: the HEADER
// cell of an aligned column right-aligns same as its body, a prose column
// stays left-aligned, empty cells sitting alongside numbers don't stop the
// column from right-aligning, and a cell that merely CONTAINS an id token
// ("TEKKEN #38") — rather than being nothing but one — does not count and
// keeps the whole column left-aligned. Single-column payloads keep the
// padding math unambiguous — a second column's own leading/trailing pad
// would fold into the gap between columns and muddy the measurement.
func TestAiTableBlockAlignedColumnRightAligns(t *testing.T) {
	cases := []struct {
		name           string
		payload        string
		shortContent   string // a cell strictly shorter than the column's widest cell
		wantRightAlign bool
	}{
		{
			name:           "prose column left-aligns",
			payload:        `{"type":"table","header":["Note"],"rows":[["hi"],["hello world"]]}`,
			shortContent:   "hi",
			wantRightAlign: false,
		},
		{
			name:           "numeric column's header right-aligns",
			payload:        `{"type":"table","header":["V"],"rows":[["7,709"],["93%"]]}`,
			shortContent:   "V",
			wantRightAlign: true,
		},
		{
			name:           "numeric column's body right-aligns",
			payload:        `{"type":"table","header":["V"],"rows":[["7,709"],["93%"]]}`,
			shortContent:   "93%",
			wantRightAlign: true,
		},
		{
			name:           "empty cells among numbers still right-align",
			payload:        `{"type":"table","header":["Ct"],"rows":[["150"],[""],["7"]]}`,
			shortContent:   "7",
			wantRightAlign: true,
		},
		{
			name:           "#id column's header right-aligns",
			payload:        `{"type":"table","header":["#"],"rows":[["#1"],["#222222222"]]}`,
			shortContent:   "#",
			wantRightAlign: true,
		},
		{
			name:           "#id column's body right-aligns",
			payload:        `{"type":"table","header":["#"],"rows":[["#1"],["#222222222"]]}`,
			shortContent:   "#1",
			wantRightAlign: true,
		},
		{
			name:           "date/time column's header right-aligns",
			payload:        `{"type":"table","header":["When"],"rows":[["19 Jul 12:00"],["5 Jun '25 12:00"]]}`,
			shortContent:   "When",
			wantRightAlign: true,
		},
		{
			name:           "date/time column's body right-aligns",
			payload:        `{"type":"table","header":["When"],"rows":[["19 Jul 12:00"],["5 Jun '25 12:00"]]}`,
			shortContent:   "19 Jul 12:00",
			wantRightAlign: true,
		},
		{
			name:           "a cell merely containing an id token stays left-aligned",
			payload:        `{"type":"table","header":["Game"],"rows":[["TEKKEN #38"],["Short"]]}`,
			shortContent:   "Short",
			wantRightAlign: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := stripANSI(plain().aiTableBlock(json.RawMessage(c.payload), 60))
			var line string
			for _, l := range strings.Split(out, "\n") {
				if strings.TrimSpace(l) == c.shortContent {
					line = l
					break
				}
			}
			if line == "" {
				t.Fatalf("no line found with content %q:\n%s", c.shortContent, out)
			}
			lead, trail := tableCellPadding(t, line)
			if c.wantRightAlign && lead <= trail {
				t.Errorf("%q: want right-aligned (lead>trail), got lead=%d trail=%d:\n%s", c.shortContent, lead, trail, out)
			}
			if !c.wantRightAlign && trail <= lead {
				t.Errorf("%q: want left-aligned (trail>lead), got lead=%d trail=%d:\n%s", c.shortContent, lead, trail, out)
			}
		})
	}
}

// TestAiTableColumnWidths table-drives aiTableColumnWidths directly — the
// @ai table block's pure, deterministic column-width allocator (owner:
// "consider number of columns and viewport") — pinning exact allocated
// widths for fixed inputs, apart from any lipgloss rendering: the
// already-fits identity case, a two-column proportional squeeze that
// lands exactly on budget, an all-rigid column set that has nothing left
// to give (numeric columns never truncate, even past budget), and a
// floor-clamp overflow that forces the deterministic settle loop to relax
// every flexible column past its own header floor toward the hard floor
// evenly.
func TestAiTableColumnWidths(t *testing.T) {
	cases := []struct {
		name        string
		natural     []int
		aligned     []bool
		headerWidth []int
		avail       int
		want        []int
	}{
		{
			name:        "already fits: identity, no squeeze at all",
			natural:     []int{14, 59},
			aligned:     []bool{false, false},
			headerWidth: []int{4, 5},
			avail:       100,
			want:        []int{14, 59},
		},
		{
			name:        "two flexible text columns squeeze proportionally to fit exactly",
			natural:     []int{14, 59},
			aligned:     []bool{false, false},
			headerWidth: []int{4, 5},
			avail:       26,
			want:        []int{13, 13},
		},
		{
			name:        "five columns: numeric + short columns stay rigid, one flexible column absorbs the whole deficit",
			natural:     []int{2, 14, 59, 3, 6},
			aligned:     []bool{true, false, false, true, true},
			headerWidth: []int{1, 4, 5, 5, 5},
			avail:       40,
			want:        []int{2, 14, 15, 3, 6},
		},
		{
			name:        "all columns rigid (numeric): natural widths survive even past budget — numeric never truncates",
			natural:     []int{20, 20},
			aligned:     []bool{true, true},
			headerWidth: []int{5, 5},
			avail:       10,
			want:        []int{20, 20},
		},
		{
			name:        "floor-clamp overflow settles evenly across tied flexible columns, past their header floor",
			natural:     []int{3, 20, 20, 20},
			aligned:     []bool{false, false, false, false},
			headerWidth: []int{1, 15, 15, 15},
			avail:       30,
			want:        []int{3, 9, 9, 9},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := aiTableColumnWidths(c.natural, c.aligned, c.headerWidth, c.avail)
			if len(got) != len(c.want) {
				t.Fatalf("got %d columns, want %d: %v", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("column %d: got %d, want %d (full: %v, want %v)", i, got[i], c.want[i], got, c.want)
				}
			}
		})
	}
}

// TestAiTableBlockFiveColumnsTightWidthNumericIntactTextTruncates pins the
// owner's two-part spec end to end: a 5-column table on a tight width
// keeps every numeric column (and any naturally-short column) at its full
// natural width — nothing there ever truncates — while the one long prose
// column (Genre) truncates with an ellipsis to absorb the squeeze.
func TestAiTableBlockFiveColumnsTightWidthNumericIntactTextTruncates(t *testing.T) {
	payload := `{"type":"table","header":["#","Game","Genre","Score","Price"],"rows":[
		["1","Doom","Action, Shooter, First-person, Sci-fi, Demon Slaying Adventure","96","59.99"],
		["2","Halo","Shooter, Sci-fi, Military Adventure, Multiplayer Campaign","91","29.99"]
	]}`
	out := stripANSI(plain().aiTableBlock(json.RawMessage(payload), 40))
	for _, want := range []string{"1", "2", "Doom", "Halo", "96", "91", "59.99", "29.99"} {
		if !strings.Contains(out, want) {
			t.Errorf("a rigid (numeric or short) column must survive at its full natural width, lost %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "…") {
		t.Errorf("the long Genre column must truncate with an ellipsis at a tight width:\n%s", out)
	}
	if strings.Contains(out, "Demon Slaying Adventure") || strings.Contains(out, "Multiplayer Campaign") {
		t.Errorf("the long Genre column must not survive in full at a tight width:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Errorf("line exceeds width 40 (%d): %q", w, line)
		}
	}
}

// TestAiTableBlockGenerousWidthMatchesUnallocatedRender pins the OTHER
// half of the same table at a generous width: the allocator never engages
// (naturals already fit), so aiTableBlock's output must be BYTE-IDENTICAL
// to constructing the exact same lipgloss/table directly off the
// untouched header/rows — no pre-truncation, no .Width() squeeze, nothing
// the allocator path could have changed.
func TestAiTableBlockGenerousWidthMatchesUnallocatedRender(t *testing.T) {
	header := []string{"#", "Game", "Genre", "Score", "Price"}
	rows := [][]string{
		{"1", "Doom", "Action, Shooter, First-person, Sci-fi, Demon Slaying Adventure", "96", "59.99"},
		{"2", "Halo", "Shooter, Sci-fi, Military Adventure, Multiplayer Campaign", "91", "29.99"},
	}
	aligned := aiTableAlignedCols(header, rows)
	reference := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(tablePurple(false))).
		BorderColumn(false).
		BorderRow(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(true).
		Wrap(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			st := lipgloss.NewStyle().Padding(0, 1)
			if col < len(aligned) && aligned[col] {
				st = st.Align(lipgloss.Right)
			}
			if row == table.HeaderRow {
				return st.Foreground(tablePurple(false)).Bold(true)
			}
			if row%2 == 1 {
				return st.Foreground(ColorDim)
			}
			return st.Foreground(ColorFaint)
		}).
		Headers(header...)
	for _, row := range rows {
		reference = reference.Row(row...)
	}
	want := reference.Render()

	payload, err := json.Marshal(struct {
		Type   string     `json:"type"`
		Header []string   `json:"header"`
		Rows   [][]string `json:"rows"`
	}{"table", header, rows})
	if err != nil {
		t.Fatal(err)
	}
	got := plain().aiTableBlock(json.RawMessage(payload), 200)
	if got != want {
		t.Errorf("a generous width must render byte-identical to the no-allocator construction:\ngot:  %q\nwant: %q", got, want)
	}
	if strings.Contains(got, "…") {
		t.Errorf("nothing should truncate at a generous width:\n%s", got)
	}
}

// TestAiTableBlockIDAndDateColumnsJoinRigidSetNeverTruncate pins the
// allocator's extended RIGID set (owner decree): an #id column and a
// date/time column classify ALIGNED exactly like a numeric column and
// therefore NEVER truncate under width pressure, even when their natural
// width sits well past aiTableRigidFloor (so it's the alignment census,
// not mere shortness, keeping them rigid here) — only the long prose
// column gives up width.
func TestAiTableBlockIDAndDateColumnsJoinRigidSetNeverTruncate(t *testing.T) {
	// The id and date cells are deliberately sized past aiTableRigidFloor
	// (8) — it's the alignment census keeping them rigid here, not mere
	// shortness — while the Released column mixes two date/time shapes
	// (the house stamp and a bare ISO date) to also exercise "cells may
	// mix families within a column".
	payload := `{"type":"table","header":["#","Game","Genre","Released"],"rows":[
		["#123456789","Doom","Action, Shooter, First-person, Sci-fi, Demon Slaying Adventure","19 Jul 12:00"],
		["#223456789","Halo","Shooter, Sci-fi, Military Adventure, Multiplayer Campaign","2026-07-19"]
	]}`
	out := stripANSI(plain().aiTableBlock(json.RawMessage(payload), 45))
	for _, want := range []string{"#123456789", "#223456789", "Doom", "Halo", "19 Jul 12:00", "2026-07-19"} {
		if !strings.Contains(out, want) {
			t.Errorf("a rigid (id or date/time) column must survive at its full natural width, lost %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "…") {
		t.Errorf("the long Genre column must truncate with an ellipsis at a tight width:\n%s", out)
	}
	if strings.Contains(out, "Demon Slaying Adventure") || strings.Contains(out, "Multiplayer Campaign") {
		t.Errorf("the long Genre column must not survive in full at a tight width:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 45 {
			t.Errorf("line exceeds width 45 (%d): %q", w, line)
		}
	}
}

// ── suggestion ───────────────────────────────────────────────────────────

func TestAiSuggestionBlockNumberedWithNote(t *testing.T) {
	payload := `{"type":"suggestion","command":"show game 12","note":"it just released"}`
	out := plain().aiSuggestionBlock(json.RawMessage(payload), 0, 60)
	if !strings.Contains(out, "1. ") || !strings.Contains(out, "show game 12") {
		t.Errorf("missing numbered command line:\n%s", out)
	}
	if !strings.Contains(out, "it just released") {
		t.Errorf("missing note line:\n%s", out)
	}
	// Note sits on its own line beneath the command.
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Errorf("note must render on a separate line:\n%q", out)
	}

	fifth := plain().aiSuggestionBlock(json.RawMessage(payload), 4, 60)
	if !strings.Contains(fifth, "5. ") {
		t.Errorf("index 4 must display as \"5.\":\n%s", fifth)
	}
}

func TestAiSuggestionBlockWithoutNote(t *testing.T) {
	payload := `{"type":"suggestion","command":"ls games"}`
	out := plain().aiSuggestionBlock(json.RawMessage(payload), 0, 60)
	if !strings.Contains(out, "ls games") {
		t.Errorf("command lost:\n%s", out)
	}
	if strings.Contains(out, "\n") {
		t.Errorf("no note means a single line: %q", out)
	}
}

func TestAiSuggestionBlockEmptyOrMalformed(t *testing.T) {
	if out := plain().aiSuggestionBlock(json.RawMessage(`{"type":"suggestion","command":""}`), 0, 60); out != "" {
		t.Errorf("blank command must render \"\": %q", out)
	}
	if out := plain().aiSuggestionBlock(json.RawMessage(`not json`), 0, 60); out != "" {
		t.Errorf("malformed JSON must render \"\": %q", out)
	}
}

// ── degrade ──────────────────────────────────────────────────────────────

func TestAiDegradeBlockRendersRawJSON(t *testing.T) {
	raw := `{"type":"holo_deck","matrix":{"rows":2},"note":"novelty"}`
	out := plain().aiDegradeBlock(json.RawMessage(raw), 60)
	for _, want := range []string{"holo_deck", "matrix", "rows", "novelty"} {
		if !strings.Contains(out, want) {
			t.Errorf("degrade block lost %q:\n%s", want, out)
		}
	}
}

func TestAiDegradeBlockEmptyRaw(t *testing.T) {
	if out := plain().aiDegradeBlock(json.RawMessage(``), 60); out != "" {
		t.Errorf("empty raw must render \"\": %q", out)
	}
	if out := plain().aiDegradeBlock(json.RawMessage(`   `), 60); out != "" {
		t.Errorf("blank raw must render \"\": %q", out)
	}
}

func TestAiDegradeBlockSurvivesInvalidJSONBytes(t *testing.T) {
	// Not valid JSON at all — json.Indent fails, so the raw bytes fall
	// through verbatim rather than erroring.
	out := plain().aiDegradeBlock(json.RawMessage(`not json at all`), 60)
	if !strings.Contains(out, "not json at all") {
		t.Errorf("invalid JSON bytes must still surface as text:\n%s", out)
	}
}
