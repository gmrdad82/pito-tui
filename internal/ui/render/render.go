// Package render turns canonical events into terminal blocks. One renderer
// per known kind, *_follow_up variants reuse their base, and everything
// else — including kinds this client has never heard of — falls back to a
// generic payload dump. Novelty must never crash: renderers decode the raw
// payload themselves and degrade on any mismatch.
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// R renders events at a fixed width. Rebuild it on terminal resize — the
// glamour renderer word-wraps at construction time.
type R struct {
	width int
	plain bool
	glam  *glamour.TermRenderer

	echoStyle    lipgloss.Style
	systemStyle  lipgloss.Style
	enhancedBar  lipgloss.Style
	errorStyle   lipgloss.Style
	dimStyle     lipgloss.Style
	confirmStyle lipgloss.Style
}

// Option configures an R.
type Option func(*R)

// WithPlain disables glamour markdown rendering (deterministic output for
// golden tests; also the safe path if glamour ever misbehaves).
func WithPlain() Option {
	return func(r *R) { r.plain = true }
}

// New builds a renderer for the given content width.
func New(width int, opts ...Option) *R {
	if width < 20 {
		width = 20
	}
	r := &R{
		width:        width,
		echoStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Width(width),
		systemStyle:  lipgloss.NewStyle().Width(width),
		enhancedBar:  lipgloss.NewStyle().Border(lipgloss.ThickBorder(), false, false, false, true).BorderForeground(lipgloss.Color("5")).PaddingLeft(1).Width(width),
		errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Width(width),
		dimStyle:     lipgloss.NewStyle().Faint(true).Width(width),
		confirmStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Width(width),
	}
	for _, opt := range opts {
		opt(r)
	}
	if !r.plain {
		// Best-effort: glamour failure downgrades to plain text forever
		// rather than erroring per event.
		if g, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(width),
		); err == nil {
			r.glam = g
		}
	}
	return r
}

// Event renders one event to a newline-terminated block.
func (r *R) Event(ev api.Event) string {
	switch ev.Kind {
	case api.KindEcho:
		return r.echo(ev)
	case api.KindSystem, api.KindSystemFollowUp:
		return r.systemStyle.Render(r.bodyText(ev)) + "\n"
	case api.KindEnhanced, api.KindEnhancedFollowUp:
		return r.enhancedBar.Render(r.bodyText(ev)) + "\n"
	case api.KindError:
		return r.errorEvent(ev)
	case api.KindThinking:
		return r.thinking(ev)
	case api.KindConfirmation, api.KindConfirmationFollowUp:
		return r.confirmation(ev)
	default:
		return r.fallback(ev)
	}
}

// Notice renders a transient dim line (web-only verb replies, local hints).
func (r *R) Notice(text string) string {
	return r.dimStyle.Render("· "+text) + "\n"
}

type textPayload struct {
	Text string `json:"text"`
	Body string `json:"body"`
	HTML bool   `json:"html"`
}

// bodyText extracts renderable text from system/enhanced-shaped payloads:
// {text} for plain copy, {body, html:true} for prerendered HTML (tags
// stripped), and markdown-ish text through glamour when available.
func (r *R) bodyText(ev api.Event) string {
	var p textPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return strings.TrimSpace(string(ev.Payload))
	}
	switch {
	case p.Body != "" && p.HTML:
		return htmlToText(p.Body)
	case p.Body != "":
		return r.markdown(p.Body)
	default:
		return r.markdown(p.Text)
	}
}

func (r *R) markdown(text string) string {
	if r.glam == nil {
		return strings.TrimSpace(text)
	}
	out, err := r.glam.Render(text)
	if err != nil {
		return strings.TrimSpace(text)
	}
	// Glamour pads with blank lines and trailing spaces; the transcript
	// owns spacing between blocks.
	return strings.TrimRight(strings.Trim(out, "\n"), " \n")
}

func (r *R) echo(ev api.Event) string {
	var p textPayload
	_ = json.Unmarshal(ev.Payload, &p)
	return r.echoStyle.Render("> "+p.Text) + "\n"
}

func (r *R) errorEvent(ev api.Event) string {
	var p struct {
		Text   string `json:"text"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(ev.Payload, &p)
	if p.Text == "" {
		return r.fallback(ev)
	}
	out := r.errorStyle.Render("✗ " + p.Text)
	if p.Detail != "" {
		out += "\n" + r.dimStyle.Render("  "+p.Detail)
	}
	return out + "\n"
}

func (r *R) thinking(ev api.Event) string {
	var p struct {
		Resolved       bool     `json:"resolved"`
		ElapsedSeconds *float64 `json:"elapsed_seconds"`
	}
	_ = json.Unmarshal(ev.Payload, &p)
	if p.Resolved && p.ElapsedSeconds != nil {
		return r.dimStyle.Render(fmt.Sprintf("thought for %.1fs", *p.ElapsedSeconds)) + "\n"
	}
	if p.Resolved {
		return r.dimStyle.Render("thought about it") + "\n"
	}
	return r.dimStyle.Render("thinking…") + "\n"
}

func (r *R) confirmation(ev api.Event) string {
	var p struct {
		Body        string `json:"body"`
		HTML        bool   `json:"html"`
		ReplyHandle string `json:"reply_handle"`
		Resolved    bool   `json:"resolved"`
		OutcomeText string `json:"outcome_text"`
	}
	_ = json.Unmarshal(ev.Payload, &p)
	body := p.Body
	if p.HTML {
		body = htmlToText(body)
	}
	if p.Resolved && p.OutcomeText != "" {
		body = p.OutcomeText
	}
	out := r.confirmStyle.Render("? " + body)
	if !p.Resolved && p.ReplyHandle != "" {
		out += "\n" + r.dimStyle.Render("  reply with "+p.ReplyHandle+" …")
	}
	return out + "\n"
}

// fallback renders any unknown kind: the kind label plus its payload,
// pretty-printed. Old clients degrade, they never crash.
func (r *R) fallback(ev api.Event) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, ev.Payload, "", "  "); err != nil {
		pretty.Reset()
		pretty.Write(ev.Payload)
	}
	label := r.dimStyle.Render("[" + ev.Kind + "]")
	return label + "\n" + r.dimStyle.Render(pretty.String()) + "\n"
}
