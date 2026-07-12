package render

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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
	payload := `{"type":"kv_table","rows":[["Released",{"v":"2026-07-10","format":"date"}],["Unparseable",{"v":"not-a-date","format":"date"}]]}`
	out := plain().aiKvTableBlock(json.RawMessage(payload), 60)
	if !strings.Contains(out, "Jul 10, 2026") {
		t.Errorf("date must render \"Jul 10, 2026\":\n%s", out)
	}
	if !strings.Contains(out, "not-a-date") {
		t.Errorf("unparseable date must fall back to the raw string:\n%s", out)
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
