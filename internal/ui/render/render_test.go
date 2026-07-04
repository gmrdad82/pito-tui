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
	if !strings.Contains(out, "#28  PITO - Inception : Vlog 006") {
		t.Errorf("row missing: %q", out)
	}
	// Right-aligned reference column: #9 pads to the width of #28.
	if !strings.Contains(out, " #9  Short one") {
		t.Errorf("right alignment broken: %q", out)
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
	if strings.Contains(out, "<img") {
		t.Errorf("html cell leaked markup:\n%s", out)
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
