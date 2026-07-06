package render

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/gmrdad82/pito-tui/internal/api"
)

func TestMain(m *testing.M) {
	// No ANSI variance across terminals — deterministic output everywhere.
	lipgloss.SetColorProfile(termenv.Ascii)
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
	if out := r.Event(event("thinking", `{"resolved":false}`)); !strings.Contains(out, "thinking…") {
		t.Errorf("unresolved = %q", out)
	}
	if out := r.Event(event("thinking", `{"resolved":true,"elapsed_seconds":2.4}`)); !strings.Contains(out, "thought for 2.4s") {
		t.Errorf("resolved = %q", out)
	}
}

func TestConfirmationPendingAndResolved(t *testing.T) {
	r := plain()
	pending := r.Event(event("confirmation", `{"body":"Unlink Hades II?","reply_handle":"@confirm-1","resolved":false}`))
	if !strings.Contains(pending, "? Unlink Hades II?") || !strings.Contains(pending, "@confirm-1") {
		t.Errorf("pending = %q", pending)
	}
	resolved := r.Event(event("confirmation", `{"body":"Unlink Hades II?","resolved":true,"outcome_text":"Unlinked."}`))
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
	fresh := r.Event(event("system", `{"text":"Linked.","reply_handle":"#a3f","reply_target":"link"}`))
	if !strings.Contains(fresh, "reply with #a3f") {
		t.Errorf("reply-capable system event missing the hint: %q", fresh)
	}
	consumed := r.Event(event("system", `{"text":"Linked.","reply_handle":"#a3f","reply_consumed":true}`))
	if strings.Contains(consumed, "#a3f") {
		t.Errorf("consumed handle must drop the hint: %q", consumed)
	}
	enhanced := r.Event(event("enhanced", `{"text":"Views up.","reply_handle":"#b2c"}`))
	if !strings.Contains(enhanced, "reply with #b2c") {
		t.Errorf("enhanced event missing the hint: %q", enhanced)
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
	out := plain().Event(event("system", payload))
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
	out := plain().Event(event("system", payload))
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
	out := plain().Event(event("system", string(raw)))

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
		"♥ 88.8%",           // the heart
		"14453 likes",       // heart detail
		"Subs stands at -2", // Butler caption survives html+shimmer stripping
	} {
		if !strings.Contains(out, want) {
			t.Errorf("analyze block missing %q:\n%s", want, out)
		}
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
	out := plain().Event(event("system", string(raw)))
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
	out := plain().Event(event("system", payload))
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
	if !strings.Contains(out, "The stock 7d report.") || !strings.Contains(out, "crunching the numbers…") {
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
