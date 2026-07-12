// The scroll-nav pills + ctrl+home / ctrl+end jumps — the TUI face of
// pito's Pito::Shell::ScrollNavComponent + pito--scroll-nav (owner
// 2026-07-13: "I need these too… implementation and look I leave it up
// to you"). Two pills float over the conversation's first and last
// visible lines: each shows one of the 50 server-authored count
// variants (render.PitoCopy.ScrollbackNav, %{count}/%{direction} +
// {singular|plural} braces resolved exactly like the web's #format),
// the kbd token, and the server's jump glyph (▲ / ▼). Rules mirrored
// from the web controller: a pill exists iff at least one turn sits
// FULLY outside the viewport on that side; both hide while a palette
// overlay is open (the suggest menu here). ctrl+home jumps to the top;
// ctrl+end re-engages follow through the house glide (the web
// smooth-scrolls; easeTowardBottom is our smooth).
//
// One deliberate deviation, documented for the record: the web picks a
// random variant per show-cycle. A Bubble Tea View must stay pure, so
// the variant is picked by HASH of conversation uuid + side — fixed per
// conversation per side. Same authored pool, same interpolation; only
// the rotation cadence is calmer.
package ui

import (
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// scrollNavBraceRe is the web's {singular|plural} token, verbatim
// (/\{([^|{}]*)\|([^{}]*)\}/g in scroll_nav_controller.js).
var scrollNavBraceRe = regexp.MustCompile(`\{([^|{}]*)\|([^{}]*)\}`)

// scrollNavText interpolates one count variant — %{count}, %{direction}
// ("above"/"below", the controller's own constants), then the braces:
// singular when count is exactly 1, else plural.
func scrollNavText(variant string, count int, direction string) string {
	s := strings.ReplaceAll(variant, "%{count}", strconv.Itoa(count))
	s = strings.ReplaceAll(s, "%{direction}", direction)
	return scrollNavBraceRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := scrollNavBraceRe.FindStringSubmatch(m)
		if count == 1 {
			return parts[1]
		}
		return parts[2]
	})
}

// scrollNavVariant picks the pill's variant index: an FNV hash of the
// conversation uuid + side, so the pick is stable while the pill lives
// and differs between the two sides (the web guarantees top ≠ bottom by
// re-rolling; the +1 salt below guarantees it by construction when the
// pool has more than one entry).
func scrollNavVariant(uuid, side string, n int) int {
	if n <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(uuid + "/" + side))
	idx := int(h.Sum32()) % n
	if idx < 0 {
		idx += n
	}
	return idx
}

// scrollNavPill renders one pill: count text (dim), the kbd token, the
// server's jump glyph. Every word but the token is pito-authored.
func scrollNavPill(variant string, count int, direction, key, glyph string, truecolor bool) string {
	return pickerDimStyle.Render(scrollNavText(variant, count, direction)) + " " +
		render.KbdBare(key, truecolor) + " " + pickerDimStyle.Render(glyph)
}

// scrollNavPills computes both pills for the current viewport ("" =
// hidden). Hidden entirely while the suggest palette overlays the
// conversation's bottom lines (the web hides its pills while the Ctrl+K
// palette / sidebar are open — same rule, our overlay).
func (m Model) scrollNavPills() (top, bottom string) {
	nav := render.PitoCopy.ScrollbackNav
	if len(nav.Count) == 0 || m.suggest != nil {
		return "", ""
	}
	above, below := m.transcript.TurnsOutside(m.contentWidth(), m.sc.YOffset(), m.sc.VisibleLineCount())
	n := len(nav.Count)
	if above > 0 {
		idx := scrollNavVariant(m.conv.UUID, "top", n)
		top = scrollNavPill(nav.Count[idx], above, "above", "ctrl+home", nav.JumpToStart, m.truecolor)
	}
	if below > 0 {
		// +1 salt: never the top pill's variant (web rule) as long as
		// the pool holds at least two entries.
		idx := scrollNavVariant(m.conv.UUID, "bottom", n)
		if top != "" && idx == scrollNavVariant(m.conv.UUID, "top", n) {
			idx = (idx + 1) % n
		}
		bottom = scrollNavPill(nav.Count[idx], below, "below", "ctrl+end", nav.JumpToEnd, m.truecolor)
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
