package render

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
)

func readAiFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// ── the full "done" fixture ─────────────────────────────────────────────

func TestAiEventFullFixtureRendersEveryBlockInOrder(t *testing.T) {
	raw := readAiFixture(t, "ai_done.json")
	out := stripANSI(plain().aiEvent(event("ai", string(raw))))

	// Every block's content survives — one unique marker per block, in the
	// order the fixture lists them.
	markers := []string{
		"great pick for your backlog", // text
		"25 Feb '22",                  // kv_table (typed date value)
		"Hades II",                    // table
		"top pick",                    // suggestion #1's note
		"ls games with genre RPG",     // suggestion #2's command
		"views/day",                   // sparkline label
		"Completion split",            // chart label
		"Metacritic",                  // score label
		"Completionist",               // ttb level legend
		"mystery_block",               // unknown-type degrade dump
	}
	cursor := 0
	for _, m := range markers {
		i := strings.Index(out[cursor:], m)
		if i < 0 {
			t.Fatalf("marker %q missing or out of order after cursor %d:\n%s", m, cursor, out)
		}
		cursor += i + len(m)
	}

	// The genre/format details each block was asked to prove.
	for _, want := range []string{
		"Genre", "Action RPG", // kv_table plain value
		"Game", "Score", "96", "91", // table header + rows
		"show game 12",                                           // suggestion #1 command
		"31h", "71h", "124h", "Extras", "Completionist", "12.5h", // ttb (current's own label
		// never renders — mirrors the web's footage tick, whose text is
		// always the hours chip, never a label)
		"94",              // score value
		"novelty", "rows", // degrade dump body
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestAiEventBlocksJoinedByExactlyOneBlankLine(t *testing.T) {
	raw := readAiFixture(t, "ai_done.json")
	var p api.AiPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	parts := plain().renderAiBlocks(p.Blocks, 56)
	if len(parts) == 0 {
		t.Fatal("fixture produced no rendered blocks")
	}
	joined := strings.Join(parts, "\n\n")
	if strings.Contains(joined, "\n\n\n") {
		t.Errorf("more than one blank line appears between some pair of blocks:\n%s", joined)
	}
	if got, want := strings.Count(joined, "\n\n"), len(parts)-1; got != want {
		t.Errorf("blank-line separators = %d, want %d (len(parts)=%d)", got, want, len(parts))
	}
}

func TestAiEventMediaBlockContributesNothing(t *testing.T) {
	var blocks []api.AiBlock
	if err := json.Unmarshal([]byte(`[
		{"type":"text","text":"before"},
		{"type":"media","entity":"vid","id":42,"variant":"thumb"},
		{"type":"text","text":"after"}
	]`), &blocks); err != nil {
		t.Fatal(err)
	}
	parts := plain().renderAiBlocks(blocks, 60)
	if len(parts) != 2 {
		t.Fatalf("media block must contribute NOTHING — want 2 parts (before/after), got %d: %v", len(parts), parts)
	}
	for _, p := range parts {
		if strings.Contains(p, "thumb") || strings.Contains(p, "variant") || strings.Contains(p, "entity") {
			t.Errorf("media block leaked into a rendered part: %q", p)
		}
	}

	// A message that is ONLY a media block renders as no blocks at all —
	// not even an empty placeholder line.
	var onlyMedia []api.AiBlock
	_ = json.Unmarshal([]byte(`[{"type":"media","entity":"vid","id":1,"variant":"cover"}]`), &onlyMedia)
	if parts := plain().renderAiBlocks(onlyMedia, 60); len(parts) != 0 {
		t.Errorf("media-only block list must render zero parts, got %v", parts)
	}
}

func TestAiEventUnknownTypeShowsDegradeDump(t *testing.T) {
	payload := `{"status":"done","blocks":[
		{"type":"holo_deck","matrix":{"rows":2},"note":"novelty"}
	]}`
	out := plain().aiEvent(event("ai", payload))
	for _, want := range []string{"holo_deck", "matrix", "rows", "novelty"} {
		if !strings.Contains(out, want) {
			t.Errorf("degrade dump missing %q:\n%s", want, out)
		}
	}
}

// ── pending status ───────────────────────────────────────────────────────

func TestAiEventPendingRendersBarStampEllipsisNoBadge(t *testing.T) {
	payload := `{"status":"pending","blocks":[{"type":"text","text":"so far so good"}],
		"model":"claude-sonnet-5","cost_amount":0.03,"cost_currency":"USD"}`
	out := plain().aiEvent(event("ai", payload))
	if !strings.Contains(out, "┃") {
		t.Errorf("pending must render inside a bar block:\n%s", out)
	}
	if !strings.Contains(out, "so far so good") {
		t.Errorf("pending must render blocks already streamed:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("pending must end on the shimmering ellipsis:\n%s", out)
	}
	if strings.Contains(out, "✦") || strings.Contains(out, "claude-sonnet-5") || strings.Contains(out, "$0.03") {
		t.Errorf("pending must never show the model/cost badge:\n%s", out)
	}
}

func TestAiEventPendingWithNoBlocksYetStillShowsEllipsis(t *testing.T) {
	out := plain().aiEvent(event("ai", `{"status":"pending","blocks":[]}`))
	if !strings.Contains(out, "…") {
		t.Errorf("an empty pending payload must still show the ellipsis:\n%s", out)
	}
}

// ── badge ────────────────────────────────────────────────────────────────

func TestAiEventBadgeRightAlignedWithModelAndCost(t *testing.T) {
	raw := readAiFixture(t, "ai_done.json")
	out := stripANSI(plain().aiEvent(event("ai", string(raw))))
	if !strings.Contains(out, "✦ claude-sonnet-5 · $0.03") {
		t.Errorf("badge must read \"✦ claude-sonnet-5 · $0.03\":\n%s", out)
	}
	var badgeLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "✦") {
			badgeLine = line
			break
		}
	}
	if badgeLine == "" {
		t.Fatalf("no badge line found:\n%s", out)
	}
	stripped := strings.TrimPrefix(badgeLine, "┃")
	if idx := strings.Index(stripped, "✦"); idx <= 0 {
		t.Errorf("badge must be right-aligned (leading padding before ✦), got %q", stripped)
	}
	if trimmed := strings.TrimRight(stripped, " "); !strings.HasSuffix(trimmed, "$0.03") {
		t.Errorf("badge must end flush at the content edge: %q", stripped)
	}
}

func TestAiEventCostNilOmitsTheDotSegment(t *testing.T) {
	payload := `{"status":"done","blocks":[{"type":"text","text":"no price yet"}],"model":"claude-sonnet-5"}`
	out := plain().aiEvent(event("ai", payload))
	if !strings.Contains(out, "✦ claude-sonnet-5") {
		t.Errorf("badge must still show the model with no cost:\n%s", out)
	}
	if strings.Contains(out, "·") {
		t.Errorf("a nil cost must drop the \"·\" segment entirely:\n%s", out)
	}
}

func TestAiEventEstimatedCostPrefixesTilde(t *testing.T) {
	payload := `{"status":"done","blocks":[{"type":"text","text":"x"}],
		"model":"claude-sonnet-5","cost_amount":0.03,"cost_currency":"USD","cost_estimated":true}`
	out := plain().aiEvent(event("ai", payload))
	if !strings.Contains(out, "✦ claude-sonnet-5 · ~$0.03") {
		t.Errorf("an estimated cost must render \"~$0.03\":\n%s", out)
	}
}

func TestAiEventReportedCostOmitsTilde(t *testing.T) {
	// cost_estimated false and cost_estimated absent must render
	// byte-identically — neither is a provider-estimated cost.
	for _, payload := range []string{
		`{"status":"done","blocks":[{"type":"text","text":"x"}],
			"model":"claude-sonnet-5","cost_amount":0.03,"cost_currency":"USD","cost_estimated":false}`,
		`{"status":"done","blocks":[{"type":"text","text":"x"}],
			"model":"claude-sonnet-5","cost_amount":0.03,"cost_currency":"USD"}`,
	} {
		out := plain().aiEvent(event("ai", payload))
		if !strings.Contains(out, "✦ claude-sonnet-5 · $0.03") {
			t.Errorf("a reported cost must render \"$0.03\" unchanged:\n%s", out)
		}
		if strings.Contains(out, "~") {
			t.Errorf("a reported cost must never carry the estimate tilde:\n%s", out)
		}
	}
}

func TestAiEventNoModelOmitsTheBadgeEntirely(t *testing.T) {
	payload := `{"status":"done","blocks":[{"type":"text","text":"x"}]}`
	out := plain().aiEvent(event("ai", payload))
	if strings.Contains(out, "✦") {
		t.Errorf("no model must mean no badge line at all:\n%s", out)
	}
}

func TestFormatAiCostNonUSDFallsBackToAmountAndCode(t *testing.T) {
	amount := 1.5
	if got, want := formatAiCost(&amount, "EUR"), "1.50 EUR"; got != want {
		t.Errorf("formatAiCost non-USD = %q, want %q", got, want)
	}
	if got, want := formatAiCost(&amount, "USD"), "$1.50"; got != want {
		t.Errorf("formatAiCost USD = %q, want %q", got, want)
	}
	if got := formatAiCost(nil, "USD"); got != "" {
		t.Errorf("formatAiCost(nil, ...) = %q, want \"\"", got)
	}
}

// ── reply affordance ─────────────────────────────────────────────────────

func TestAiEventConsumedHandleDropsTheAffordance(t *testing.T) {
	fresh := `{"status":"done","blocks":[{"type":"text","text":"x"}],"reply_handle":"#ai-9"}`
	out := plain().aiEvent(event("ai", fresh))
	if !strings.Contains(out, "shift+r") || !strings.Contains(out, "#ai-9") {
		t.Errorf("an unconsumed handle must show the reply affordance:\n%s", out)
	}

	consumed := `{"status":"done","blocks":[{"type":"text","text":"x"}],"reply_handle":"#ai-9","reply_consumed":true}`
	out = plain().aiEvent(event("ai", consumed))
	if strings.Contains(out, "shift+r") || strings.Contains(out, "#ai-9") {
		t.Errorf("a consumed handle must drop the reply affordance:\n%s", out)
	}
}

// ── suggestion numbering ────────────────────────────────────────────────

func TestAiEventSuggestionNumberingIncrementsAcrossBlocks(t *testing.T) {
	payload := `{"status":"done","blocks":[
		{"type":"suggestion","command":"show game 12"},
		{"type":"suggestion","command":"ls games"}
	]}`
	out := plain().aiEvent(event("ai", payload))
	iFirst := strings.Index(out, "1. ")
	iSecond := strings.Index(out, "2. ")
	if iFirst < 0 || iSecond < 0 || iSecond <= iFirst {
		t.Errorf("suggestion numbering must increment 1. then 2.:\n%s", out)
	}
	if !strings.Contains(out, "show game 12") || !strings.Contains(out, "ls games") {
		t.Errorf("both suggestion commands must render:\n%s", out)
	}
}

// ── decode failure ───────────────────────────────────────────────────────

func TestAiEventUndecodablePayloadFallsBack(t *testing.T) {
	out := plain().aiEvent(event("ai", `"just a string"`))
	if !strings.Contains(out, "[ai]") {
		t.Errorf("a non-object payload must degrade through the fallback:\n%s", out)
	}
}

// ── score adapter ────────────────────────────────────────────────────────

func TestAiScoreBlockClampsAboveHundred(t *testing.T) {
	out := stripANSI(plain().aiScoreBlock(json.RawMessage(`{"value":150,"label":"Metacritic"}`), 60))
	if !strings.Contains(out, "100") {
		t.Errorf("a value above 100 must clamp to 100:\n%s", out)
	}
	if strings.Contains(out, "150") {
		t.Errorf("the raw out-of-range value must not leak through:\n%s", out)
	}
	if !strings.Contains(out, "Metacritic") {
		t.Errorf("label lost:\n%s", out)
	}
}

func TestAiScoreBlockClampsBelowZero(t *testing.T) {
	out := plain().aiScoreBlock(json.RawMessage(`{"value":-40}`), 60)
	if !strings.Contains(out, "0") {
		t.Errorf("a negative value must clamp to 0:\n%s", out)
	}
	if strings.Contains(out, "-40") {
		t.Errorf("the raw negative value must not leak through:\n%s", out)
	}
}

func TestAiScoreBlockDefaultsLabelAndDegradesOnMalformed(t *testing.T) {
	if out := plain().aiScoreBlock(json.RawMessage(`{"value":80}`), 60); !strings.Contains(out, "Score") {
		t.Errorf("a missing label must default to \"Score\":\n%s", out)
	}
	if out := plain().aiScoreBlock(json.RawMessage(`not json`), 60); out != "" {
		t.Errorf("malformed JSON must render \"\": %q", out)
	}
}

// ── ttb adapter ──────────────────────────────────────────────────────────

func TestAiTtbBlockRendersLevelsLabelsAndHours(t *testing.T) {
	payload := `{"label":"Time to beat","levels":[
		{"label":"Main","hours":31},
		{"label":"Extras","hours":71},
		{"label":"Completionist","hours":124}
	],"current":{"label":"Recorded","hours":12.5}}`
	out := stripANSI(plain().aiTtbBlock(json.RawMessage(payload), 56))
	for _, want := range []string{
		"Time to beat",
		"Main", "31h",
		"Extras", "71h",
		"Completionist", "124h",
		"12.5h",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ttb block missing %q:\n%s", want, out)
		}
	}
	// Bracketed fill, like every other bar this engine draws.
	if !strings.Contains(out, "[") || !strings.Contains(out, "]") {
		t.Errorf("ttb bar must be bracketed:\n%s", out)
	}
}

func TestAiTtbBlockOverlongLegendTruncatesRatherThanWrapping(t *testing.T) {
	// Caller-supplied labels (unlike the game preset's short main/extras/
	// completionist strings) can run long enough that the legend row
	// alone exceeds the column — it must truncate with an ellipsis
	// in-place, never spill a raw, unprefixed continuation line into the
	// surrounding chrome.
	payload := `{"levels":[
		{"label":"A Very Long Main Story Level Name","hours":10},
		{"label":"An Even Longer Extras Level Name","hours":20},
		{"label":"The Longest Completionist Level Name Of All","hours":30}
	]}`
	out := plain().aiTtbBlock(json.RawMessage(payload), 56)
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 56 {
			t.Errorf("ttb line exceeds width 56 (%d): %q", w, line)
		}
	}
	if !strings.Contains(out, "…") {
		t.Errorf("an overlong legend must truncate with an ellipsis:\n%s", out)
	}
}

func TestAiTtbBlockWithoutCurrentStillRenders(t *testing.T) {
	out := plain().aiTtbBlock(json.RawMessage(`{"levels":[{"label":"Main","hours":10}]}`), 56)
	if !strings.Contains(out, "Main") || !strings.Contains(out, "10h") {
		t.Errorf("single-level ttb without current must still render:\n%s", out)
	}
}

func TestAiTtbBlockWholeVsFractionalCurrentHours(t *testing.T) {
	whole := stripANSI(plain().aiTtbBlock(json.RawMessage(`{"levels":[{"label":"Main","hours":10}],"current":{"hours":5}}`), 56))
	if !strings.Contains(whole, "5h") || strings.Contains(whole, "5.0h") {
		t.Errorf("a whole-number current must drop the decimal (\"5h\"):\n%s", whole)
	}
	half := stripANSI(plain().aiTtbBlock(json.RawMessage(`{"levels":[{"label":"Main","hours":10}],"current":{"hours":2.5}}`), 56))
	if !strings.Contains(half, "2.5h") {
		t.Errorf("a fractional current must keep one decimal (\"2.5h\"):\n%s", half)
	}
}

func TestAiTtbBlockEmptyOrMalformedDegrades(t *testing.T) {
	if out := plain().aiTtbBlock(json.RawMessage(`not json`), 56); out != "" {
		t.Errorf("malformed JSON must render \"\": %q", out)
	}
	if out := plain().aiTtbBlock(json.RawMessage(`{"levels":[]}`), 56); out != "" {
		t.Errorf("no levels must render \"\": %q", out)
	}
	if out := plain().aiTtbBlock(json.RawMessage(`{"levels":[{"label":"x","hours":0}]}`), 56); out != "" {
		t.Errorf("an all-zero level list must render \"\" (nothing to scale against): %q", out)
	}
}

func TestAiTtbBlockCapsAtFourLevels(t *testing.T) {
	payload := `{"levels":[
		{"label":"A","hours":1},{"label":"B","hours":2},
		{"label":"C","hours":3},{"label":"D","hours":4},{"label":"E","hours":5}
	]}`
	out := plain().aiTtbBlock(json.RawMessage(payload), 56)
	if strings.Contains(out, "E") {
		t.Errorf("a 5th level must be dropped (cap 4):\n%s", out)
	}
	if !strings.Contains(out, "D") {
		t.Errorf("the 4th level must still render:\n%s", out)
	}
}

// ── sanity: width sanity check via lipgloss ─────────────────────────────

func TestAiEventBadgeStaysWithinWidth(t *testing.T) {
	raw := readAiFixture(t, "ai_done.json")
	out := plain().aiEvent(event("ai", string(raw)))
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line exceeds the renderer's own width (%d > 60): %q", w, line)
		}
	}
}

func TestAiEventRendersTheLiveDevFixture(t *testing.T) {
	// ai_live.json is a REAL @ai answer captured from dev (2026-07-11,
	// deepseek-v4-flash-free): text + table + text blocks, cost 0.00 USD.
	raw, err := os.ReadFile("testdata/ai_live.json")
	if err != nil {
		t.Fatal(err)
	}
	out := plain().Event(event("ai", string(raw)))
	for _, want := range []string{"┃"} {
		if !strings.Contains(out, want) {
			t.Errorf("live fixture render missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\"type\"") {
		t.Errorf("live fixture degraded to raw JSON:\n%s", out)
	}
}
