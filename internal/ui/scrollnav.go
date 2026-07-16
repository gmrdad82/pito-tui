// The scroll-nav pills + ctrl+home / ctrl+end jumps — the TUI face of
// pito's Pito::Shell::ScrollNavComponent + pito--scroll-nav (owner
// 2026-07-13: "I need these too… implementation and look I leave it up
// to you"). Two pills float over the conversation's first and last
// visible lines: each shows one fixed, server-authored copy line
// (render.PitoCopy.ScrollbackNav.{Before,After}, %{count} interpolated
// client-side), the kbd token, and the server's jump glyph (▲ / ▼).
// Rules mirrored from the web controller: a pill exists iff at least one
// turn sits FULLY outside the viewport on that side; both hide while a
// palette overlay is open (the suggest menu here). ctrl+home jumps to
// the top; ctrl+end re-engages follow through the house glide (the web
// smooth-scrolls; easeTowardBottom is our smooth).
//
// Owner contract 2026-07-13: the 50-variant random-pick pool is retired
// — one clear string per side, identical everywhere (web + tui), so the
// deliberate hash-pick deviation this file used to document no longer
// applies. The copy WORDS render in the default (white) foreground, not
// the muted/dim style; only the kbd token and the glyph keep their own
// styling.
package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// scrollNavText interpolates the one token a fixed pill string carries,
// %{count}. Counts 1–10 render exactly; above ten, the short form "10+"
// (owner 2026-07-15) — the web controller's #format clamps identically.
func scrollNavText(format string, count int) string {
	shown := strconv.Itoa(count)
	if count > 10 {
		shown = "10+"
	}
	return strings.ReplaceAll(format, "%{count}", shown)
}

// scrollNavPill renders one pill: the copy text in the default (white)
// foreground — deliberately NOT pickerDimStyle — then the kbd token,
// then the server's jump glyph in the dim style. Order per the owner's
// contract: copy text, kbd token, glyph. KbdBare already pads both of
// its own sides with a single space, so no extra spacing is added here.
func scrollNavPill(text, key, glyph string, truecolor bool) string {
	return text + render.KbdBare(key, truecolor) + pickerDimStyle.Render(glyph)
}

// scrollNavPills computes both pills for the current viewport ("" =
// hidden). Hidden entirely while the suggest palette overlays the
// conversation's bottom lines (the web hides its pills while the Ctrl+K
// palette / sidebar are open — same rule, our overlay). COPY LAW: an
// older pito ref without a side's string degrades that pill to absent,
// never a fabricated word.
func (m Model) scrollNavPills() (top, bottom string) {
	if m.suggest != nil {
		return "", ""
	}
	nav := render.PitoCopy.ScrollbackNav
	above, below := m.transcript.TurnsOutside(m.contentWidth(), m.sc.YOffset(), m.sc.VisibleLineCount())
	if above > 0 && nav.Before != "" {
		top = scrollNavPill(scrollNavText(nav.Before, above), "ctrl+home", nav.JumpToStart, m.truecolor)
	}
	if below > 0 && nav.After != "" {
		bottom = scrollNavPill(scrollNavText(nav.After, below), "ctrl+end", nav.JumpToEnd, m.truecolor)
	}
	return top, bottom
}

// paintScrollNavOverlay lays the pills over the viewport body's first
// and last lines, right-aligned with a one-cell margin — floating over
// the content like the web's fixed-position pills, never reflowing it.
func paintScrollNavOverlay(body, top, bottom string, width int) string {
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return body
	}
	paint := func(line, pill string) string {
		pw := lipgloss.Width(pill)
		keep := width - pw - 2
		if keep < 0 {
			return line
		}
		base := ansi.Truncate(line, keep, "")
		if pad := keep - lipgloss.Width(base); pad > 0 {
			base += strings.Repeat(" ", pad)
		}
		return base + " " + pill
	}
	if top != "" {
		lines[0] = paint(lines[0], top)
	}
	if bottom != "" {
		lines[len(lines)-1] = paint(lines[len(lines)-1], bottom)
	}
	return strings.Join(lines, "\n")
}
