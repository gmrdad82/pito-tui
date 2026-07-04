package render

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/gmrdad82/pito-tui/internal/api"
)

func TestMain(m *testing.M) {
	// No ANSI variance across terminals — deterministic output everywhere.
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func event(kind string, payload string) api.Event {
	return api.Event{ID: 1, TurnID: 1, Kind: kind, Payload: json.RawMessage(payload)}
}

func plain() *R { return New(60, WithPlain()) }

func TestEcho(t *testing.T) {
	out := plain().Event(event("echo", `{"text":"show game 5"}`))
	if !strings.Contains(out, "> show game 5") {
		t.Errorf("echo = %q", out)
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
		"paragraphs":  {"<p>one</p><p>two</p>", "one\ntwo"},
		"inline join": {"<span>a</span><em>b</em>c", "abc"},
		"entities":    {"<p>fish &amp; chips</p>", "fish & chips"},
		"list items":  {"<ul><li>x</li><li>y</li></ul>", "x\ny"},
		"not html":    {"plain words", "plain words"},
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
