package render

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/gmrdad82/pito-tui/internal/api"
)

func renderFixture(t *testing.T, name string, width int) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return New(width, WithTruecolor(true)).Event(api.Event{ID: 1, Kind: "system", Payload: json.RawMessage(raw)})
}

func stripANSI(s string) string {
	return regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(s, "")
}

func TestShowGameCardAnatomy(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "show_game_full", 100))

	// kv rows: stats + shinies folded in, platform tokens from img alts.
	for _, want := range []string{
		"Stats", "365 Views · 17 Likes · 2 Comments", "Shinies",
		"Title", "Ghosts 'n Goblins Resurrection", "ID", "#1",
		"Platforms", " PS ", " Switch ", " Xbox ", " Steam ",
		"Developer", "Capcom", "Price", "39.99",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("card missing %q:\n%s", want, out)
		}
	}
	// Bars: score chip 75 inside brackets; TTB footage chip; legend row.
	if !regexp.MustCompile(`\[=+75\|=+\]`).MatchString(out) {
		t.Errorf("score bar with 75 chip left of tick missing:\n%s", out)
	}
	if !strings.Contains(out, "12.5h") {
		t.Errorf("TTB footage chip missing:\n%s", out)
	}
	for _, want := range []string{"| main", "| extras", "| completionist", "31h", "71h", "124h"} {
		if !strings.Contains(out, want) {
			t.Errorf("TTB row missing %q:\n%s", want, out)
		}
	}
	// Description on its own: label line then prose.
	if !strings.Contains(out, "Description") || !strings.Contains(out, "The legendary platforming series") {
		t.Errorf("description block missing:\n%s", out)
	}
	// Price wears gold Mario coins beside the number (payload count).
	if !strings.Contains(out, "●") || !strings.Contains(out, "39.99") {
		t.Errorf("gold coins + price missing:\n%s", out)
	}
	// No image leakage of any kind.
	for _, banned := range []string{"<img", "◉", "sync game #1", "=" + strings.Repeat("=", 200)} {
		if strings.Contains(out, banned) {
			t.Errorf("image/placeholder residue %q leaked:\n%s", banned, out)
		}
	}
}

func TestShowCardBracketAndTickAlignment(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "show_game_freebie", 100))
	lines := strings.Split(out, "\n")
	var barCols []int
	var valueLine, barLine string
	for i, line := range lines {
		if idx := strings.Index(line, "["); idx >= 0 && strings.Contains(line, "=") {
			barCols = append(barCols, idx)
			if strings.Contains(line, "h") && i+1 < len(lines) { // the TTB bar
				barLine = line
				valueLine = lines[i+1]
			}
		}
	}
	if len(barCols) != 2 {
		t.Fatalf("want exactly 2 bars, got %d:\n%s", len(barCols), out)
	}
	if barCols[0] != barCols[1] {
		t.Errorf("score and TTB brackets misaligned: cols %v", barCols)
	}
	// Pillar hour values center under their ticks (at-end anchors right).
	ticks := []int{}
	for i, r := range barLine {
		if r == '|' {
			ticks = append(ticks, i)
		}
	}
	for _, m := range regexp.MustCompile(`\d+h`).FindAllStringIndex(valueLine, -1) {
		center := m[0] + (m[1]-m[0])/2
		ok := false
		for _, tc := range ticks {
			if center >= tc-2 && center <= tc+2 {
				ok = true
			}
		}
		if !ok {
			t.Errorf("value at col %d not under any tick %v:\n%s\n%s", center, ticks, barLine, valueLine)
		}
	}
}

func TestShowVidAndChannelCards(t *testing.T) {
	withTrueColor(t)
	vid := stripANSI(renderFixture(t, "show_vid", 100))
	for _, want := range []string{"YouTube ID", "7y3R403XtDE", "Length", "16:21", "Visibility", "Public", "Description"} {
		if !strings.Contains(vid, want) {
			t.Errorf("vid card missing %q:\n%s", want, vid)
		}
	}
	// v1.6.0: tags left the kv grid for their own hairline section —
	// they must render as a labeled block, not vanish (pito db74203f).
	if !strings.Contains(vid, "Tags") || !strings.Contains(vid, "talking head") {
		t.Errorf("tags section missing:\n%s", vid)
	}
	tagIdx, descIdx := strings.Index(vid, "Tags"), strings.Index(vid, "Description")
	if tagIdx < descIdx {
		t.Errorf("tags section must follow the description (tags=%d desc=%d)", tagIdx, descIdx)
	}
	if strings.Contains(vid, "[=") {
		t.Errorf("vid card must not grow bars:\n%s", vid)
	}
	channel := stripANSI(renderFixture(t, "show_channel", 100))
	for _, want := range []string{"Handle", "@gmrdad82", "Subs", "YouTube Channel", "youtube.com/@gmrdad82"} {
		if !strings.Contains(channel, want) {
			t.Errorf("channel card missing %q:\n%s", want, channel)
		}
	}
}

// TestShowVidDescriptionParagraphsAreSpaced covers W2 finding 1: the
// fixture's description div is "whitespace-pre-wrap" prose with the
// author's blank lines marking 4 paragraphs (no <p> tags — verified
// against testdata/show_vid.json directly), and the web renders 4 spaced
// blocks. htmlToText used to replace every embedded "\n" with a space
// (correct for ordinary flow text, wrong for pre-wrap), gluing all 4 into
// one wall of text. Each paragraph anchor below sits on that paragraph's
// first wrapped line, so wrapping at width 100 doesn't split the match.
func TestShowVidDescriptionParagraphsAreSpaced(t *testing.T) {
	// Parses the fixture directly (parseDetailCard → descriptionText),
	// under the full Event() pipeline's per-line left border ("│" on
	// every scrollback row) which is orthogonal to this fix and would
	// otherwise have to be stripped out of every blank-line check.
	raw, err := os.ReadFile("testdata/show_vid.json")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	card, ok := parseDetailCard(p.Body)
	if !ok {
		t.Fatal("show_vid fixture no longer parses as a detail card")
	}
	// W2 finding 1: the description div is "whitespace-pre-wrap" prose —
	// no <p> tags (verified against the fixture directly) — with the
	// author's blank lines marking 4 paragraphs. htmlToText used to
	// replace every embedded "\n" with a space (correct for ordinary flow
	// text, wrong for pre-wrap), gluing all 4 into one wall of text; the
	// fixed flattening keeps them as one blank line per paragraph break.
	paragraphs := strings.Split(card.descText, "\n\n")
	if len(paragraphs) != 4 {
		t.Fatalf("want 4 blank-line-separated paragraphs, got %d:\n%q", len(paragraphs), card.descText)
	}
	wantPrefixes := []string{
		"PITO is a minimalist",
		"No dashboards with forty tabs",
		"Built for me first",
		"#pito #gmrdad82",
	}
	for i, want := range wantPrefixes {
		if !strings.HasPrefix(paragraphs[i], want) {
			t.Errorf("paragraph %d = %q, want prefix %q", i, paragraphs[i], want)
		}
	}
	// The web's rendered card shows the same 4 blocks with a blank line
	// between — confirm the full pipeline (word-wrap, left border and
	// all) doesn't re-glue them by requiring at least one wholly blank
	// (border-only) line between each paragraph's rendered block.
	out := renderFixture(t, "show_vid", 100)
	lines := strings.Split(stripANSI(out), "\n")
	var blanks int
	inDesc := false
	for _, l := range lines {
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "┃"))
		if strings.Contains(l, "Description") {
			inDesc = true
			continue
		}
		if strings.Contains(l, "Tags") {
			break
		}
		if inDesc && trimmed == "" {
			blanks++
		}
	}
	if blanks < 3 {
		t.Errorf("want at least 3 blank lines between the description's 4 rendered paragraphs, got %d:\n%s", blanks, stripANSI(out))
	}
}

func TestKvZebraWrapsValuesInColumn(t *testing.T) {
	withTrueColor(t)
	long := strings.Repeat("wordy ", 30)
	payload := `{"body":"x","html":true}`
	_ = payload
	card := &detailCard{pairs: [][2]string{
		{"Title", "short"},
		{"Notes", long},
	}}
	out := stripANSI(New(80, WithTruecolor(true)).detailCard(card))
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("line exceeds width (%d > 80): %q", w, line)
		}
	}
	// Continuation lines align into the value column, not the left edge.
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("long value should wrap: %d lines\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[len(lines)-1], strings.Repeat(" ", 8)) {
		t.Errorf("continuation not indented into value column: %q", lines[len(lines)-1])
	}
}

// TestKvRowsZebraAlignsToCharmStyling pins the Charm restyle on kvRows
// (owner 2026-07-12 "align to Charm"): keys stay ColorDim regardless of
// row parity, VALUES alternate ColorFaint(241, even rows)/ColorDim(245,
// odd rows) — and no background SGR survives, the old plum stripe is gone.
func TestKvRowsZebraAlignsToCharmStyling(t *testing.T) {
	withTrueColor(t)
	r := New(80, WithTruecolor(true))
	pairs := [][2]string{
		{"Title", "Alpha"},
		{"Genre", "Bravo"},
		{"Dev", "Charlie"},
	}
	out := r.kvRows(pairs, 80, true)
	if strings.Contains(out, "48;2;") || strings.Contains(out, "48;5;") {
		t.Errorf("kv rows must carry no background SGR at all (the plum zebra is gone):\n%q", out)
	}
	if strings.Count(out, "\x1b[38;5;245m ") != 3 {
		t.Errorf("every key must stay ColorDim regardless of row parity:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;241mAlpha\x1b[m") {
		t.Errorf("even value row must use ColorFaint (241):\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;245mBravo\x1b[m") {
		t.Errorf("odd value row must use ColorDim (245):\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;241mCharlie\x1b[m") {
		t.Errorf("even value row must use ColorFaint (241):\n%q", out)
	}

	// zebra=false (the Stats/Shinies block) keeps plain, unstyled values.
	plainOut := r.kvRows(pairs, 80, false)
	if strings.Contains(plainOut, "\x1b[38;5;241m") || strings.Contains(plainOut, "\x1b[38;5;245mAlpha") {
		t.Errorf("non-zebra kv rows must not gain gray alternation:\n%q", plainOut)
	}
}

func TestScoreBarReusable(t *testing.T) {
	withTrueColor(t)
	r := New(80, WithTruecolor(true))

	low := stripANSI(r.ScoreBar("Score ", 12, 60))
	if !regexp.MustCompile(`\[=*\|12=+\]`).MatchString(low) {
		t.Errorf("low score chip must sit right of the tick: %q", low)
	}
	high := stripANSI(r.ScoreBar("Score ", 93, 60))
	if !regexp.MustCompile(`\[=+93\|=*\]`).MatchString(high) {
		t.Errorf("high score chip must sit left of the tick: %q", high)
	}
	muted := stripANSI(r.ScoreBar("Score ", -1, 60))
	if strings.Contains(muted, "|") || !strings.Contains(muted, "=") {
		t.Errorf("muted bar must be a bare fill: %q", muted)
	}
	for _, bar := range []string{low, high, muted} {
		if w := lipgloss.Width(bar); w > 60 {
			t.Errorf("bar exceeds width (%d > 60): %q", w, bar)
		}
	}
}

func TestFreeGameWearsTheStar(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "show_game_freebie", 100))
	if !strings.Contains(out, "★") || !strings.Contains(out, "0.00") {
		t.Errorf("free game must wear the gold star beside 0.00:\n%s", out)
	}
	if strings.Contains(out, "●") {
		t.Errorf("free game must not stack coins:\n%s", out)
	}
}

func TestPlatformChipsStayPlainInTables(t *testing.T) {
	withTrueColor(t)
	// A list row with a platform-icon cell: the table gets the short
	// label as plain text, never marker runes or chip styling.
	payload := `{"body":"x","html":true,
		"table_heading": ["#", "Game", "Platform"],
		"table_rows":[{"cells":[
			{"text": "#1", "class": "text-right"},
			{"text": "Ghosts", "class": ""},
			{"html": true, "text": "<span class=\"pito-platform-icons\"><img class=\"pito-platform-icon\" alt=\"PlayStation\" src=\"/x.svg\"><img class=\"pito-platform-icon\" alt=\"Steam\" src=\"/y.svg\"></span>", "class": ""}
		]}]}`
	out := stripANSI(New(100, WithTruecolor(true)).Event(event("system", payload)))
	if !strings.Contains(out, "PS Steam") {
		t.Errorf("table platform cell must show short labels:\n%s", out)
	}
	if strings.ContainsRune(out, ChipStart) || strings.ContainsRune(out, CoinMark) {
		t.Errorf("marker runes leaked into the table:\n%s", out)
	}
}

func TestCoinsStackTightWithOneGapBeforeNumber(t *testing.T) {
	withTrueColor(t)
	out := stripANSI(renderFixture(t, "show_game_full", 100))
	if strings.Contains(out, "● ●") {
		t.Errorf("coins must stack with no gaps:\n%s", out)
	}
	if !strings.Contains(out, "● 39.99") {
		t.Errorf("one space between the last coin and the number:\n%s", out)
	}
	free := stripANSI(renderFixture(t, "show_game_freebie", 100))
	if !strings.Contains(free, "★ 0.00") {
		t.Errorf("star wants the same single gap:\n%s", free)
	}
}
