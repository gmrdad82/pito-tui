package render

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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
