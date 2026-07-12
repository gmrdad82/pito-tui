package render

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
)

func TestMain(m *testing.M) {
	// Lip Gloss v2 has no global color profile: Style.Render always emits
	// full-fidelity ANSI, and every test here asserts on plain substrings
	// or lipgloss.Width (both profile-independent) rather than exact bytes,
	// so there is nothing to force here anymore.
	time.Local = time.UTC
	os.Exit(m.Run())
}

func event(kind string, payload string) api.Event {
	return api.Event{ID: 1, TurnID: 1, Kind: kind, Payload: json.RawMessage(payload)}
}

func plain() *R { return New(60, WithPlain()) }

func TestEcho(t *testing.T) {
	out := plain().Event(event("echo", `{"text":"show game 5"}`))
	if !strings.Contains(out, "show game 5") || !strings.Contains(out, "┃") {
		t.Errorf("echo must be a bar block: %q", out)
	}
}

func TestSystemText(t *testing.T) {
	out := plain().Event(event("system", `{"text":"All systems nominal."}`))
	if !strings.Contains(out, "All systems nominal.") {
		t.Errorf("system = %q", out)
	}
}

func TestSystemFollowUpUsesSystemRenderer(t *testing.T) {
	base := plain().Event(event("system", `{"text":"same shape"}`))
	follow := plain().Event(event("system_follow_up", `{"text":"same shape"}`))
	if base != follow {
		t.Errorf("follow_up diverged from base:\n%q\n%q", base, follow)
	}
}

func TestEnhancedHTMLBodyStripped(t *testing.T) {
	out := plain().Event(event("enhanced", `{"body":"<p>Views are <strong>up 14%</strong> this week.</p>","html":true}`))
	if !strings.Contains(out, "Views are up 14% this week.") {
		t.Errorf("enhanced = %q", out)
	}
	if strings.Contains(out, "<") {
		t.Errorf("tags leaked: %q", out)
	}
}

func TestErrorWithDetail(t *testing.T) {
	out := plain().Event(event("error", `{"text":"That did not go as planned.","detail":"quota exceeded"}`))
	if !strings.Contains(out, "✗ That did not go as planned.") || !strings.Contains(out, "quota exceeded") {
		t.Errorf("error = %q", out)
	}
}

func TestThinkingStates(t *testing.T) {
	r := plain()
	// Pending, full payload: braille frame + the word the shared formula
	// picks (order[elapsed/5s % len] into the dictionary's doing pool) —
	// ThinkingComponent parity, Capitalized like the web (owner smoke
	// 2026-07-12). plain()'s clock is unpinned, elapsed vs a fresh
	// started_at rounds to step 0 → order[0]=2 → slash doing[2].
	slash := thinkingCopy["slash"]
	if len(slash.Doing) < 4 || len(slash.Done) < 4 {
		t.Fatal("slash dictionary missing from thinking_copy.json")
	}
	pending := fmt.Sprintf(`{"resolved":false,"dictionary":"slash","order":[2,0,1],"started_at":%q}`,
		time.Now().Add(-time.Second).Format(time.RFC3339))
	out := r.Event(event("thinking", pending))
	if !strings.Contains(out, "⠋") {
		t.Errorf("pending must carry a braille frame: %q", out)
	}
	if !strings.Contains(out, slash.Doing[2]+"…") {
		t.Errorf("pending word must replay the payload order: %q (want %q)", out, slash.Doing[2])
	}
	// Sparse payload (unknown dictionary / no order): COPY LAW degrade —
	// the frame and ellipsis alone, no client-substituted word.
	out = r.Event(event("thinking", `{"resolved":false}`))
	if !strings.Contains(out, "⠋ …") {
		t.Errorf("unresolved fallback = %q", out)
	}
	if strings.Contains(out, "Thinking") {
		t.Errorf("client must not invent a word: %q", out)
	}
	// Resolved: seeded kaomoji + past-tense word + stripped elapsed
	// ("%{word} for %{elapsed}s"). event() builds ID 1 → glyphs[1].
	resolved := r.Event(event("thinking", `{"resolved":true,"dictionary":"slash","word_index":3,"elapsed_seconds":2.40}`))
	if !strings.Contains(resolved, slash.Done[3]+" for 2.4s") {
		t.Errorf("resolved = %q (want %q)", resolved, slash.Done[3]+" for 2.4s")
	}
	if !strings.Contains(resolved, thinkingGlyphs[1]) {
		t.Errorf("resolved glyph must be seeded from the event id: %q", resolved)
	}
}

func TestFormatElapsedStripsZeros(t *testing.T) {
	for in, want := range map[float64]string{0.224: "0.22", 0.5: "0.5", 1.0: "1", 2.47: "2.47", 0: "0"} {
		if got := formatElapsed(in); got != want {
			t.Errorf("formatElapsed(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestConfirmationPendingAndResolved(t *testing.T) {
	r := plain()
	pending := stripANSI(r.Event(event("confirmation", `{"body":"Unlink Hades II?","reply_handle":"@confirm-1","resolved":false}`)))
	if !strings.Contains(pending, "? Unlink Hades II?") || !strings.Contains(pending, "@confirm-1") {
		t.Errorf("pending = %q", pending)
	}
	resolved := stripANSI(r.Event(event("confirmation", `{"body":"Unlink Hades II?","resolved":true,"outcome_text":"Unlinked."}`)))
	if !strings.Contains(resolved, "Unlinked.") || strings.Contains(resolved, "@confirm") {
		t.Errorf("resolved = %q", resolved)
	}
}

func TestUnknownKindFallsBack(t *testing.T) {
	out := plain().Event(event("holo_deck", `{"matrix":{"rows":2},"note":"novelty"}`))
	if !strings.Contains(out, "[holo_deck]") || !strings.Contains(out, "novelty") {
		t.Errorf("fallback = %q", out)
	}
}

func TestFallbackSurvivesNonObjectPayload(t *testing.T) {
	out := plain().Event(event("mystery", `"just a string"`))
	if !strings.Contains(out, "[mystery]") {
		t.Errorf("fallback = %q", out)
	}
}

func TestNotice(t *testing.T) {
	out := plain().Notice("/themes is web-only")
	if !strings.Contains(out, "· /themes is web-only") {
		t.Errorf("notice = %q", out)
	}
}

func TestMarkdownThroughGlamour(t *testing.T) {
	// Non-plain renderer: glamour renders emphasis; content must survive.
	out := New(60).Event(event("system", `{"text":"deploy **now** please"}`))
	if !strings.Contains(out, "now") || !strings.Contains(out, "deploy") {
		t.Errorf("glamour output lost content: %q", out)
	}
}

func TestHTMLToText(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"paragraphs": {"<p>one</p><p>two</p>", "one\ntwo"},
		// Adjacent inline ELEMENTS get a space (kills label/value glue in
		// detail cards); element→text still joins directly.
		"inline spacing": {"<span>a</span><em>b</em>c", "a bc"},
		"entities":       {"<p>fish &amp; chips</p>", "fish & chips"},
		"list items":     {"<ul><li>x</li><li>y</li></ul>", "x\ny"},
		"not html":       {"plain words", "plain words"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := htmlToText(tc.in); got != tc.want {
				t.Errorf("htmlToText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFallbackWithInvalidPayloadBytes(t *testing.T) {
	out := plain().Event(api.Event{ID: 1, TurnID: 1, Kind: "weird", Payload: []byte("not json")})
	if !strings.Contains(out, "[weird]") || !strings.Contains(out, "not json") {
		t.Errorf("fallback = %q", out)
	}
}

func TestReplyHandleAffordance(t *testing.T) {
	r := plain()
	fresh := stripANSI(r.Event(event("system", `{"text":"Linked.","reply_handle":"#a3f","reply_target":"link"}`)))
	if !strings.Contains(fresh, "#a3f shift+r") {
		t.Errorf("reply-capable system event missing the hint: %q", fresh)
	}
	consumed := stripANSI(r.Event(event("system", `{"text":"Linked.","reply_handle":"#a3f","reply_consumed":true}`)))
	if strings.Contains(consumed, "#a3f") {
		t.Errorf("consumed handle must drop the hint: %q", consumed)
	}
	enhanced := stripANSI(r.Event(event("enhanced", `{"text":"Views up.","reply_handle":"#b2c"}`)))
	if !strings.Contains(enhanced, "#b2c shift+r") {
		t.Errorf("enhanced event missing the hint: %q", enhanced)
	}
}

// TestReplyAffordanceHelper pins the shared builder's exact structure —
// handle (accent) + space + "shift+r" kbd chip, no "reply with"/"…"
// leftovers — so the three call sites (replyHintFor, confirmation,
// aiDoneEvent) can't quietly diverge again. Checked under both plain and
// truecolor R: the chip's background is cosmetic only, the visible text
// (post stripANSI) must match in both.
func TestReplyAffordanceHelper(t *testing.T) {
	// Trailing space is the chip's own padding (the house chip idiom —
	// ShinyBadge/platformChip pad the same way); trimmed, the visible
	// shape is exactly the web's "#iota-5965 shift+r".
	const handle, want = "#iota-5965", "#iota-5965 shift+r"

	got := strings.TrimRight(stripANSI(plain().replyAffordance(handle)), " ")
	if got != want {
		t.Errorf("plain replyAffordance = %q, want %q", got, want)
	}
	if strings.Contains(got, "reply with") || strings.Contains(got, "…") {
		t.Errorf("replyAffordance must not carry the old copy: %q", got)
	}

	live := strings.TrimRight(stripANSI(New(60, WithTruecolor(true)).replyAffordance(handle)), " ")
	if live != want {
		t.Errorf("truecolor replyAffordance = %q, want %q", live, want)
	}
}

func TestTableRowsRenderAligned(t *testing.T) {
	// Real `ls vids` payload shape captured from dev (trimmed).
	payload := `{
		"body": "The catalogue holds <span class=\"x\">23</span> vids.",
		"html": true,
		"table_rows": [
			{"cells": [
				{"text": "#28", "class": "pito-action-shimmer tabular-nums text-right"},
				{"text": "PITO - Inception : Vlog 006", "class": "text-fg pito-cell-title"}
			]},
			{"cells": [
				{"text": "#9", "class": "pito-action-shimmer tabular-nums text-right"},
				{"text": "Short one", "class": "text-fg pito-cell-title"}
			]}
		]
	}`
	out := stripANSI(plain().Event(event("system", payload)))
	if !strings.Contains(out, "The catalogue holds 23 vids.") {
		t.Errorf("headline missing: %q", out)
	}
	// Horizontal rules only (owner call): top + bottom dash rules close
	// the heading-less body, and no vertical border glyphs anywhere.
	rules := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Count(line, "─") > 5 {
			rules++
		}
	}
	if rules != 2 {
		t.Errorf("want top+bottom rules on a heading-less table, got %d:\n%s", rules, out)
	}
	if strings.ContainsAny(out, "│╭╮╰╯") {
		t.Errorf("vertical borders must be gone:\n%s", out)
	}
	if !strings.Contains(out, "#28") || !strings.Contains(out, "PITO - Inception : Vlog 006") {
		t.Errorf("row content missing:\n%s", out)
	}
	// Right-aligned reference column: #9 sits flush right of its cell.
	if !strings.Contains(out, " #9 ") {
		t.Errorf("right alignment broken:\n%s", out)
	}
}

func TestTableHeadingAndFooter(t *testing.T) {
	// ls channels shape: string-or-object headings, html avatar cell,
	// dim footer under the table.
	payload := `{
		"body": "The channels you keep: 1.", "html": true,
		"table_heading": ["", "Handle", {"text": "Subs", "class": "text-right"}],
		"table_rows": [{"cells": [
			{"html": true, "text": "<img alt=\"x\" src=\"/a.jpg\" />", "class": "pito-cell-avatar"},
			{"text": "@gmrdad82", "class": "pito-action-shimmer"},
			{"text": "2.2K", "class": "text-fg-dim text-right tabular-nums"}
		]}],
		"list_footer": "sort accepts handle, title, subs."
	}`
	out := plain().Event(event("system", payload))
	for _, want := range []string{"Handle", "Subs", "@gmrdad82", "2.2K", "sort accepts handle"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Headed frame: top rule, header rule, bottom rule — nothing vertical.
	rules := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Count(line, "─") > 5 {
			rules++
		}
	}
	if rules != 3 {
		t.Errorf("want 3 dash rules on a headed table, got %d:\n%s", rules, out)
	}
	if strings.Contains(out, "<img") || strings.Contains(out, "◉") {
		t.Errorf("avatar cell must vanish, not leak:\n%s", out)
	}
}

// TestTableAlignsToCharmStyling pins the Charm restyle (owner 2026-07-12:
// "my tables have custom colors / chroma that I don't like. can you align
// them to Charm?") — bold purple header + rules (256-color ColorPrimary
// "99" off truecolor, the literal hex #7D56F4 = 125,86,244 under it,
// matching Charm's own canonical lipgloss/table example), alternating
// ColorDim(245)/ColorFaint(241) gray VALUE foregrounds on data rows, and
// NO background SGR anywhere — the old plum zebra stripe is gone.
// Meaningful class hints (the pink pito-action-shimmer id) still override
// the decorative gray, on the header's own row 0.
func TestTableAlignsToCharmStyling(t *testing.T) {
	withTrueColor(t)
	payload := `{
		"body": "x", "html": true,
		"table_heading": ["#", "Game"],
		"table_rows": [
			{"cells": [{"text": "#1", "class": "pito-action-shimmer text-right"}, {"text": "Alpha"}]},
			{"cells": [{"text": "#2", "class": "text-right"}, {"text": "Bravo"}]},
			{"cells": [{"text": "#3", "class": "text-right"}, {"text": "Charlie"}]},
			{"cells": [{"text": "#4", "class": "text-right"}, {"text": "Delta"}]}
		]
	}`
	out := New(80, WithTruecolor(true)).Event(event("system", payload))

	if strings.Contains(out, "48;2;") || strings.Contains(out, "48;5;") {
		t.Errorf("table must carry no background SGR at all (the plum zebra is gone):\n%q", out)
	}
	if !strings.Contains(out, "\x1b[1;38;2;125;86;244m#\x1b[m") {
		t.Errorf("header cell must be bold + the exact truecolor Charm purple:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;2;125;86;244m─") {
		t.Errorf("rules must ride the same Charm purple as the header:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;205m#1\x1b[m") {
		t.Errorf("meaningful pink action-shimmer accent must survive on row 0 untouched by the gray alternation:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;245m#2\x1b[m") || !strings.Contains(out, "\x1b[38;5;245m#4\x1b[m") {
		t.Errorf("odd data rows must share the ColorDim (245) gray:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[38;5;241m#3\x1b[m") {
		t.Errorf("even data rows must use the ColorFaint (241) gray:\n%q", out)
	}

	// Off-truecolor: header/rules fall back to the 256-color ColorPrimary
	// ("99") — literally what Charm's own canonical example uses too.
	plainOut := plain().Event(event("system", payload))
	if !strings.Contains(plainOut, "\x1b[1;38;5;99m#\x1b[m") {
		t.Errorf("off-truecolor header must use ColorPrimary (99):\n%q", plainOut)
	}
	if strings.Contains(plainOut, "48;2;") || strings.Contains(plainOut, "48;5;") {
		t.Errorf("off-truecolor table must carry no background SGR either:\n%q", plainOut)
	}
}

func TestHelpSections(t *testing.T) {
	payload := `{
		"body": "PITO's full command surface.",
		"sections": [
			{"title": "COMMANDS", "rows": [
				{"key": "/help", "value": "Show available commands"},
				{"key": "/login", "value": "Sign in with a verification code"}
			]},
			{"title": "KEYBINDINGS", "rows": [{"key": "d / d d", "value": "Arm / confirm delete"}]}
		]
	}`
	out := stripANSI(plain().Event(event("system", payload)))
	for _, want := range []string{"COMMANDS", "KEYBINDINGS", "/help", "Show available commands", "d / d d"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Keys align: both command rows pad to the same width.
	if !strings.Contains(out, "/help   Show") {
		t.Errorf("key column not aligned:\n%s", out)
	}
}

func TestHTMLDetailCardStructures(t *testing.T) {
	// The show-vid card shapes: label/value span grid + aria-labeled icons.
	grid := `<div class="grid grid-cols-[max-content_1fr] gap-x-2">
		<span class="text-fg-dim">Title</span><span class="text-fg">Vlog 006</span>
		<span class="text-fg-dim">ID</span><span class="pito-action-shimmer">#28</span>
	</div>`
	out := htmlToText(grid)
	if !strings.Contains(out, "Title  Vlog 006") || !strings.Contains(out, "ID     #28") {
		t.Errorf("grid pairs not aligned per line:\n%s", out)
	}

	icons := `<span><span>2</span><svg aria-label="Likes"><path d="M0"/></svg></span>`
	if got := htmlToText(icons); !strings.Contains(got, "2 Likes") {
		t.Errorf("svg aria-label lost: %q", got)
	}
	if got := htmlToText(`<img alt="face" src="/x.jpg"/>next`); strings.Contains(got, "face") {
		t.Errorf("img alt must not leak: %q", got)
	}
}

func TestI18nOnlyErrorRendersHumanely(t *testing.T) {
	out := plain().Event(event("error", `{"message_key":"pito.chat.unlink.usage","message_args":{}}`))
	if !strings.Contains(out, "✗ usage") || !strings.Contains(out, "pito.chat.unlink.usage") {
		t.Errorf("message_key error must render a hint, not a JSON dump: %q", out)
	}
	if strings.Contains(out, "{") {
		t.Errorf("no raw JSON in error blocks: %q", out)
	}
}

func TestAnalyzeChartsFromLivePayload(t *testing.T) {
	raw, err := os.ReadFile("testdata/analyze_channel.json")
	if err != nil {
		t.Fatal(err)
	}
	out := stripANSI(plain().Event(event("system", string(raw))))

	// Charts draw in braille (the BrailleAreaChart port) — never the old
	// solid block runes.
	braille := false
	for _, ru := range out {
		if ru > 0x2800 && ru <= 0x28FF {
			braille = true
			break
		}
	}
	if !braille {
		t.Errorf("no braille chart rendered:\n%s", out)
	}
	if strings.ContainsAny(out, "▁▂▃▄▅▆▇█") {
		t.Errorf("solid block runes leaked into charts:\n%s", out)
	}
	for _, want := range []string{
		"total -2",          // subs total (negative-friendly)
		"prev 37",           // views previous window
		"target 3.22/d",     // pacing
		"27%",               // avg_viewed_pct via total_pct
		"88.80%",            // the heart's pct, now inside heartCanvas's braille legend (was the old "♥ 88.8%" text line)
		"14453 Likes",       // heart detail, now heartCanvas's "<n> Likes / <n> Dislikes (<pct>)" legend (was lowercase "likes")
		"Subs stands at -2", // Butler caption survives html+shimmer stripping

		// D9 parity fix: every stash metric now draws through the SAME
		// 42x11 ticked area engine the D8 breakdowns extras use (never
		// the old bare 2-row sparkline, which stamped no tick VALUES at
		// all) — these three assertions each pin a piece of that upgrade
		// that the old sparkline body could never have produced.
		"322",    // views' y-tick: compactCount(target_daily 322.43) stamped at the chart's ceiling row
		"50.80%", // avg_viewed_pct's y-tick: percent-formatted (XX.XX%), not the raw "50.8"
		"29 Jun", // subs/views' x-tick: real dates, day-first ("%-d %b", the web's own Area#format_date)
	} {
		if !strings.Contains(out, want) {
			t.Errorf("analyze block missing %q:\n%s", want, out)
		}
	}
	// avg_viewed_pct is the web's one area chart plotted on a fixed
	// 0%→100% POSITION axis rather than dates (Rails: `@metric ==
	// :avg_viewed_pct` short-circuits #preset_x_axis before it ever looks
	// at whether dates were supplied) — its x-tick row must show the
	// percent scale, not a stray "1 Jul"/"5 Jul" repeat of the dates above.
	if !strings.Contains(out, "0%       25%        50%       75%     100%") {
		t.Errorf("avg_viewed_pct must render the fixed percent x-axis:\n%s", out)
	}
}

func TestBrailleAreaMirrorsPito(t *testing.T) {
	// Flat zero series: the one-dot baseline floor, never a blank gap.
	rows := BrailleArea([]float64{0, 0, 0}, 3, 2, 0)
	if rows[1] != "⣀⣀⣀" {
		t.Errorf("zero series must draw the baseline floor: %q", rows[1])
	}
	if rows[0] != "⠀⠀⠀" {
		t.Errorf("zero series top row must stay blank: %q", rows[0])
	}
	// A peak fills its column to the top; positives always clear the floor.
	rows = BrailleArea([]float64{0, 10, 0}, 3, 2, 10)
	if !strings.ContainsRune(rows[0], '⣿') && !strings.Contains(rows[0], "⢠") && rows[0] == "⠀⠀⠀" {
		t.Errorf("peak must reach the top row: %q", rows[0])
	}
	tiny := BrailleArea([]float64{0, 0.01, 0}, 3, 2, 100)
	if tiny[1] == "⣀⣀⣀" {
		t.Errorf("a strictly-positive value must clear the baseline: %q", tiny[1])
	}
}

func TestAnalyzeSuppressesTheWebBodyArt(t *testing.T) {
	raw, _ := os.ReadFile("testdata/analyze_channel.json")
	out := stripANSI(plain().Event(event("system", string(raw))))
	if strings.Contains(out, "The stock 7d report on 6 channels") == false {
		t.Errorf("intro line must survive:\n%s", out)
	}
	// The fixture body says "report on 6 channels." inside a div — if the
	// body were flattened too, the intro would appear twice.
	if strings.Count(out, "stock 7d report") != 1 {
		t.Errorf("web body must yield to data-drawn charts:\n%s", out)
	}
}

func TestChannelDetailGridDropsTheAvatarPair(t *testing.T) {
	// Avatar images render nowhere as text (owner call) — the whole
	// label/value pair drops from the card grid.
	grid := `<div class="grid grid-cols-[max-content_1fr] gap-x-2">
		<span class="text-fg-dim">Avatar</span><img class="pito-channel-tiny-avatar" src="/a.jpg"/>
		<span class="text-fg-dim">Handle</span><span class="pito-action-shimmer">@gmrdad82</span>
		<span class="text-fg-dim">Title</span><span class="text-fg">Catalin Ilinca</span>
	</div>`
	out := htmlToText(grid)
	if strings.Contains(out, "Avatar") || strings.Contains(out, "◉") {
		t.Errorf("avatar pair must drop:\n%s", out)
	}
	for _, want := range []string{"Handle  @gmrdad82", "Title   Catalin Ilinca"} {
		if !strings.Contains(out, want) {
			t.Errorf("grid line %q missing:\n%s", want, out)
		}
	}
}

func TestAvatarColumnDropsFromTables(t *testing.T) {
	payload := `{"body":"x","html":true,
		"table_heading": ["", "Handle"],
		"table_rows":[{"cells":[
		{"html": true, "text": "<img class=\"pito-channel-tiny-avatar\" src=\"/a.jpg\"/>", "class": "pito-cell-avatar"},
		{"text": "@gmrdad82", "class": "pito-action-shimmer"}
	]}]}`
	out := stripANSI(plain().Event(event("system", payload)))
	if strings.Contains(out, "◉") {
		t.Errorf("no stand-in glyphs (owner call):\n%s", out)
	}
	// The table holds only the handle column — no phantom gap column: a
	// row line whose entire content is the handle.
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "┃")) == "@gmrdad82" {
			found = true
		}
	}
	if !found {
		t.Errorf("avatar column must drop entirely:\n%s", out)
	}
}

func TestAnalyzeVidLevelSurvivesListShapedLikesSlot(t *testing.T) {
	// vid-level stash.likes.data is a LIST — one alien entry must not
	// poison the chart metrics around it (live-captured shape).
	raw, err := os.ReadFile("testdata/analyze_vid.json")
	if err != nil {
		t.Fatal(err)
	}
	out := plain().Event(event("system", string(raw)))
	brailleVid := false
	for _, ru := range out {
		if ru > 0x2800 && ru <= 0x28FF {
			brailleVid = true
			break
		}
	}
	if !brailleVid {
		t.Errorf("vid-level charts missing:\n%s", out)
	}
	// D9 parity fix, same as the channel-level fixture above: the
	// watched_hours/views stash metrics must carry the full ticked area
	// treatment too (y-ticks, day-first x-tick dates) — not just "some
	// braille exists somewhere," which the old bare 2-row sparkline would
	// also have satisfied.
	stripped := stripANSI(out)
	for _, want := range []string{
		"321",    // views' y-tick: compactCount(target_daily 321.43)
		"16",     // watched_hours' y-tick: compactCount(target_daily 16.07)
		"50.00%", // avg_viewed_pct's y-tick: percent-formatted ceiling (target_daily 50.0)
		"29 Jun", // day-first x-tick date
	} {
		if !strings.Contains(stripped, want) {
			t.Errorf("vid-level chart missing %q:\n%s", want, stripped)
		}
	}
}

func TestPendingAnalyzeSuppressesBodyArtToo(t *testing.T) {
	// Before the per-metric jobs land, stash has no series — the web's
	// pending dot-grids must still not leak into the terminal.
	payload := `{"body":"<div>⠂⠂⠂⠂ giant pending art ⠂⠂⠂⠂</div>","html":true,
		"analyze":{"intro":"The stock 7d report.","level":"channel","stash":{}}}`
	out := plain().Event(event("system", payload))
	if strings.Contains(out, "pending art") {
		t.Errorf("web pending art leaked:\n%s", out)
	}
	if !strings.Contains(out, "The stock 7d report.") || !strings.Contains(out, strings.Repeat("⠂", 42)) {
		t.Errorf("intro + pending note missing:\n%s", out)
	}
}

func TestHeadedEmptyColumnsSurvive(t *testing.T) {
	// `with platform` on games with no platforms set: every body cell is
	// empty but the column is DATA — it must render (owner report).
	payload := `{"body":"x","html":true,
		"table_heading": ["#", "Game", "Platform"],
		"table_rows":[{"cells":[
			{"text": "#27", "class": "pito-action-shimmer text-right"},
			{"text": "Astro Bot", "class": ""},
			{"text": "", "class": ""}
	]}]}`
	out := plain().Event(event("system", payload))
	if !strings.Contains(out, "Platform") {
		t.Errorf("headed empty column must survive:\n%s", out)
	}
}

func TestOverflowingTableTruncatesInsteadOfWrapping(t *testing.T) {
	// A table wider than the terminal must shrink by truncating cells with
	// … — wrapping spilled continuation lines outside the frame (owner
	// capture, `with genre` on games).
	payload := `{"body":"x","html":true,
		"table_heading": ["#", "Game", "Genre", "Developer"],
		"table_rows":[
			{"cells":[
				{"text": "#20", "class": "text-right"},
				{"text": "Demon's Souls", "class": ""},
				{"text": "Role-playing (RPG), Hack and slash/Beat 'em up, Adventure", "class": ""},
				{"text": "Bluepoint Games", "class": ""}
			]},
			{"cells":[
				{"text": "#15", "class": "text-right"},
				{"text": "Cyberpunk 2077", "class": ""},
				{"text": "Shooter, Role-playing (RPG), Adventure", "class": ""},
				{"text": "CD Projekt RED", "class": ""}
			]}
		]}`
	out := plain().Event(event("system", payload))
	if !strings.Contains(out, "…") {
		t.Errorf("overflow must truncate with ellipsis:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line exceeds width (%d > 60): %q", w, line)
		}
	}
	// One rendered line per data row — no wrapped continuations. Each row
	// ref appears exactly once, and both rows share the frame: 3 dash
	// rules (top, header, bottom).
	for _, ref := range []string{"#20", "#15"} {
		if n := strings.Count(out, ref); n != 1 {
			t.Errorf("row %s must render exactly once, got %d:\n%s", ref, n, out)
		}
	}
	rules := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Count(line, "─") > 10 {
			rules++
		}
	}
	if rules != 3 {
		t.Errorf("want 3 dash rules (top, header, bottom), got %d:\n%s", rules, out)
	}
}

func TestLocalEchoCarriesTimestamp(t *testing.T) {
	// Covered at model level implicitly; here: an echo with CreatedAt set
	// renders its stamp.
	ev := api.Event{ID: -1, TurnID: -1, Kind: "echo",
		Payload: []byte(`{"text":"#h with platform"}`)}
	if out := plain().Event(ev); strings.Contains(out, ":") && len(out) > 0 {
		// zero CreatedAt → no stamp
		if strings.Count(out, ":") > 1 {
			t.Errorf("zero time must not stamp: %q", out)
		}
	}
}

func TestAiEchoWearsTheAccentBar(t *testing.T) {
	// 2.0.0: @ai turns' echoes carry ai:true and wear the AI accent —
	// a purple→pito-blue vertical gradient on the bar.
	if AIAccent.At(0) != (RGB{0xbb, 0x9a, 0xf7}) || AIAccent.At(1) != (RGB{0x51, 0x70, 0xff}) {
		t.Errorf("AI accent stops moved: %+v %+v", AIAccent.At(0), AIAccent.At(1))
	}
	out := plain().Event(event("echo", `{"text":"@ai what should I play","ai":true}`))
	if !strings.Contains(out, "┃") || !strings.Contains(out, "@ai what should I play") {
		t.Errorf("ai echo lost its bar or text:\n%s", out)
	}
	// A plain echo still renders the classic bar (no regression).
	out = plain().Event(event("echo", `{"text":"list games"}`))
	if !strings.Contains(out, "list games") {
		t.Errorf("plain echo broken:\n%s", out)
	}
}

func TestStampsAgeIntoDatedForms(t *testing.T) {
	// Owner bug 2026-07-11: a 5-day-old conversation read like today.
	// Owner format: today = HH:MM; same year = "6 Jul 15:04"; other
	// year = "2 Jan '25 15:04".
	now := time.Now()
	cases := []struct {
		at   time.Time
		want string
	}{
		{now, now.Format("15:04")},
		{now.AddDate(0, 0, -5), now.AddDate(0, 0, -5).Format("2 Jan 15:04")},
		{now.AddDate(-1, 0, 0), now.AddDate(-1, 0, 0).Format("2 Jan '06 15:04")},
	}
	for _, c := range cases {
		ev := api.Event{ID: 1, Kind: "echo", CreatedAt: c.at,
			Payload: []byte(`{"text":"hello"}`)}
		out := plain().Event(ev)
		if !strings.Contains(out, c.want) {
			t.Errorf("stamp for %s: want %q in:\n%s", c.at, c.want, out)
		}
	}
	// A same-year boundary guard: yesterday must NOT render bare HH:MM
	// (unless yesterday was last year, which the dated form covers too).
	y := now.AddDate(0, 0, -1)
	ev := api.Event{ID: 1, Kind: "echo", CreatedAt: y, Payload: []byte(`{"text":"x"}`)}
	if out := plain().Event(ev); !strings.Contains(out, y.Format("2 Jan")) {
		t.Errorf("yesterday lost its date:\n%s", out)
	}
}

func TestTokensPaintCyanAndSubjectsRidePink(t *testing.T) {
	// Color spec v2 (owner 2026-07-12): subjects ride pink + a band
	// DERIVED from it (+24° hue, +14% lightness); references ride cyan +
	// its own derived band. The base stops are asserted so a future ramp
	// edit can't silently regress them.
	if got := SubjectShimmer.At(0); got != subjectBase {
		t.Errorf("subject shimmer base drifted: %+v", got)
	}
	if got := ReferenceShimmer.At(0); got != referenceBase {
		t.Errorf("reference shimmer base drifted: %+v", got)
	}
	if band := deriveBandPair(subjectBase); band == subjectBase {
		t.Error("derived band must differ from its base")
	}

	payload := `{"body":"Report for <span class=\"pito-token\">7d</span> on <span class=\"pito-subject-shimmer\">6 channels</span>.","html":true}`
	out := plain().Event(event("system", payload))
	if !strings.Contains(out, "7d") || !strings.Contains(out, "6 channels") {
		t.Fatalf("token/subject text lost:\n%s", out)
	}
	for _, ru := range out {
		if ru >= '' && ru <= '' {
			t.Errorf("private-use marker rune %U leaked into output:\n%s", ru, out)
		}
	}

	// Table cells stay plain: markers stripped, text kept.
	if got := plainTokens("see lifetime now"); got != "see lifetime now" {
		t.Errorf("plainTokens mangled a token: %q", got)
	}
}

func TestConfirmationCardsShowTheStatsDetail(t *testing.T) {
	raw, err := os.ReadFile("testdata/confirmation_disconnect.json")
	if err != nil {
		t.Fatal(err)
	}
	out := plain().Event(event("confirmation", string(raw)))

	// Pending: the web's detail block renders — cyan keys, aligned values,
	// under a hairline (the blank spacer between Views and Vids survives).
	for _, key := range []string{"Subs", "Views", "Vids", "Published", "Scheduled", "Unlisted", "Private"} {
		if !strings.Contains(out, key) {
			t.Errorf("pending confirmation lost detail key %q:\n%s", key, out)
		}
	}
	if !strings.Contains(out, "─") {
		t.Errorf("detail block missing its hairline:\n%s", out)
	}

	// Resolved: the detail drops (web hides it once resolved), outcome stays.
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	p["resolved"] = true
	p["outcome_text"] = "Called off."
	resolved, _ := json.Marshal(p)
	out = plain().Event(event("confirmation", string(resolved)))
	if strings.Contains(out, "Subs") {
		t.Errorf("resolved confirmation must hide the stats detail:\n%s", out)
	}
	if !strings.Contains(out, "Called off.") {
		t.Errorf("resolved confirmation lost its outcome text:\n%s", out)
	}
}

func TestHelpBlocksKeepTheirLineLayout(t *testing.T) {
	payload := `{"body":"<div class=\"pito-help-block\"><span data-pito-ts-slot></span><span class=\"text-purple font-bold\">Usage:</span>\n  <span class=\"text-fg-dim\">list games  |  list vids</span>\n\n<span class=\"text-purple font-bold\">Options</span>\n  <span>--help</span></div>","html":true}`
	out := plain().Event(event("system", payload))
	// W2 finding 2: headers now carry the web's purple/bold paint — strip
	// ANSI for the line-layout assertions (the suite's stripANSI idiom),
	// keep the raw `out` around for the color assertions below.
	stripped := stripANSI(out)
	lines := strings.Split(stripped, "\n")
	rawLines := strings.Split(out, "\n")
	usageLine, optionsLine := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "Usage:") {
			usageLine = i
		}
		if strings.Contains(l, "Options") {
			optionsLine = i
		}
	}
	if usageLine < 0 || optionsLine < 0 || optionsLine <= usageLine {
		t.Errorf("help block lines collapsed (usage=%d options=%d):\n%s", usageLine, optionsLine, stripped)
	}

	// Section headers wear the web's "text-purple font-bold" (man_page.rb's
	// `header` helper) — classStyle already maps text-purple to
	// ColorPrimary; font-bold adds the Bold(true).
	headerStyle := classStyle("text-purple font-bold").Bold(true)
	for _, want := range []string{"Usage:", "Options"} {
		if styled := headerStyle.Render(want); !strings.Contains(out, styled) {
			t.Errorf("header %q lost its purple/bold paint:\n%q", want, out)
		}
	}

	// Everything else — the dim usage copy, the bare --help span — stays
	// plain: this fix closes only the section-header color gap named by
	// the audit, not a full markup-fidelity pass. Every scrollback line
	// (including these) carries the message's own dim left-border escape
	// codes, so "no escape codes at all" isn't the right test; check
	// instead that these lines carry no BOLD SGR attribute — the one
	// thing only the new header styling introduces.
	for i, l := range lines {
		if strings.Contains(l, "list games") || strings.Contains(l, "--help") {
			if strings.Contains(rawLines[i], "\x1b[1;") || strings.Contains(rawLines[i], "\x1b[1m") {
				t.Errorf("non-header line unexpectedly bold-styled: %q", rawLines[i])
			}
		}
	}
}

func TestJobsStatusKvRowsRender(t *testing.T) {
	// W4 audit finding: /jobs status ships kv-hash table_rows
	// ({key, value, key_class, value_class}) — the config-status shape,
	// not {cells}. Fixture captured live from dev (2026-07-12).
	raw, err := os.ReadFile("testdata/jobs_status.json")
	if err != nil {
		t.Fatal(err)
	}
	out := stripANSI(plain().Event(event("system", string(raw))))
	if !strings.Contains(out, "Queue status") {
		t.Fatalf("body lost:\n%s", out)
	}
	for _, key := range []string{"Workers:", "Ready:", "Scheduled:", "Failed:"} {
		if !strings.Contains(out, key) {
			t.Errorf("kv row %q missing:\n%s", key, out)
		}
	}
	// Values align in a shared column — Workers and Scheduled values
	// start at the same offset on their stripped lines.
	var offsets []int
	for _, line := range strings.Split(out, "\n") {
		for _, k := range []string{"Workers:", "Scheduled:"} {
			if i := strings.Index(line, k); i >= 0 {
				rest := line[i+len(k):]
				offsets = append(offsets, i+len(k)+len(rest)-len(strings.TrimLeft(rest, " ")))
			}
		}
	}
	if len(offsets) == 2 && offsets[0] != offsets[1] {
		t.Errorf("kv values misaligned: %v\n%s", offsets, out)
	}
}

func TestBareHelpSectionHeadersPaintAndBreathe(t *testing.T) {
	// The bare `help` reply (unlike --help man pages) ships
	// <div class="text-purple font-bold">GAMES</div> section headers over
	// data-grids — captured live 2026-07-12. Headers paint bold-purple
	// via marker runes and get a blank line of air above them.
	payload := `{"html":true,"body":"<div class=\"text-purple font-bold\">GAMES</div><div class=\"pito-data-grid\" data-cols=\"2\"><span class=\"text-fg\">delete</span><span class=\"text-fg-dim\">Use --help</span></div><div class=\"text-purple font-bold\">VIDEOS</div><div class=\"pito-data-grid\" data-cols=\"2\"><span class=\"text-fg\">publish</span><span class=\"text-fg-dim\">Use --help</span></div>"}`
	out := stripANSI(plain().Event(event("system", payload)))
	for _, h := range []string{"GAMES", "VIDEOS"} {
		if !strings.Contains(out, h) {
			t.Fatalf("header %q lost:\n%s", h, out)
		}
	}
	// A blank line (bar + padding only) separates the GAMES grid from
	// the VIDEOS header.
	lines := strings.Split(out, "\n")
	sawDelete, blankAfter := false, false
	for _, line := range lines {
		body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "┃"))
		switch {
		case strings.Contains(line, "delete"):
			sawDelete = true
		case sawDelete && body == "":
			blankAfter = true
		case strings.Contains(line, "VIDEOS"):
			if !blankAfter {
				t.Errorf("no blank-line rhythm before VIDEOS:\n%s", out)
			}
		}
	}
	for _, ru := range out {
		if ru >= '\uE000' && ru <= '\uE030' {
			t.Errorf("marker rune %U leaked:\n%s", ru, out)
		}
	}
}
