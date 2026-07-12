// Mouse text selection with auto-copy — owner order 2026-07-12: "in
// Claude Code CLI I have mouse scroll on the conversation and select
// without shift drag… auto-copy to clipboard when selecting… and a small
// toast notification… implement".
//
// With the terminal's mouse mode on (View.MouseMode, the wheel's price),
// the terminal no longer does native selection — so the app does, the
// way Claude Code does: a left-button drag tracks a stream-style
// selection over the composed frame (first line from the anchor to its
// end, middle lines whole, last line up to the cursor — a terminal's own
// selection shape), painted as reverse-video while the button is down.
// On release the selected text is stripped of styling, trimmed, and
// pushed to the system clipboard via OSC 52 (tea.SetClipboard); a small
// toast confirms it with a line from the owner's own 50-word pool
// (pito.copy.tui.clipboard, mirrored by tools/copygen — COPY LAW: the
// client never authors the words; an empty pool degrades to the ✓ glyph
// alone). The toast rides the status row's empty left padding for
// toastTicks (~2.4s), so nothing shifts.
package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// toastTicksTotal: how long the copied-toast stays — ~2.4s of fast-loop
// ticks, long enough to read a quip, short enough to never nag.
const toastTicksTotal = int64(2400 * time.Millisecond / shimmerTick)

func (m Model) onMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	m.selecting = true
	m.selAnchorX, m.selAnchorY = mouse.X, mouse.Y
	m.selCursorX, m.selCursorY = mouse.X, mouse.Y
	return m, nil
}

func (m Model) onMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if !m.selecting {
		return m, nil
	}
	mouse := msg.Mouse()
	m.selCursorX, m.selCursorY = mouse.X, mouse.Y
	return m, nil
}

func (m Model) onMouseRelease(msg tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	if !m.selecting {
		return m, nil
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	m.selCursorX, m.selCursorY = mouse.X, mouse.Y
	m.selecting = false // clear BEFORE re-rendering: the copy source must not carry the highlight
	text := selectionText(ansi.Strip(m.viewContent()), m.selAnchorX, m.selAnchorY, m.selCursorX, m.selCursorY)
	if strings.TrimSpace(text) == "" {
		return m, nil // a plain click, or a drag over blank cells — no copy, no toast
	}
	m.toastText = render.ClipboardToastLine(int(m.aliveTicks) + len(text))
	m.toastTicks = toastTicksTotal
	return m, tea.Batch(tea.SetClipboard(text), m.animate())
}

// selectionText slices the stream-style selection out of a PLAIN
// (ANSI-stripped) frame. Coordinates are terminal cells, zero-based;
// anchor and cursor may arrive in either order. Each line is trimmed of
// trailing whitespace, mirroring what a terminal emulator copies.
func selectionText(plainFrame string, ax, ay, cx, cy int) string {
	sy, sx, ey, ex := ay, ax, cy, cx
	if cy < ay || (cy == ay && cx < ax) {
		sy, sx, ey, ex = cy, cx, ay, ax
	}
	lines := strings.Split(plainFrame, "\n")
	if sy >= len(lines) {
		return ""
	}
	if ey >= len(lines) {
		ey, ex = len(lines)-1, -1 // -1 = to end of line
	}
	var out []string
	for y := sy; y <= ey; y++ {
		line := lines[y]
		runes := []rune(line)
		lo, hi := 0, len(runes)
		if y == sy {
			lo = min(sx, len(runes))
		}
		if y == ey && ex >= 0 {
			hi = min(ex+1, len(runes))
		}
		if lo > hi {
			lo = hi
		}
		out = append(out, strings.TrimRight(string(runes[lo:hi]), " "))
	}
	return strings.Join(out, "\n")
}

// paintSelectionOverlay inverts the in-flight selection on the composed
// frame — the same post-processing seam the scroll thumb and star field
// use, so no section learns anything about selections. The inverted run
// is re-rendered from the plain text (styles inside it flatten while
// highlighted, exactly like Claude Code's own selection).
func (m Model) paintSelectionOverlay(body string) string {
	sy, sx, ey, ex := m.selAnchorY, m.selAnchorX, m.selCursorY, m.selCursorX
	if ey < sy || (ey == sy && ex < sx) {
		sy, sx, ey, ex = ey, ex, sy, sx
	}
	lines := strings.Split(body, "\n")
	invert := lipgloss.NewStyle().Reverse(true)
	for y := sy; y <= ey && y < len(lines); y++ {
		line := lines[y]
		width := lipgloss.Width(line)
		lo := 0
		if y == sy {
			lo = sx
		}
		hi := width
		if y == ey {
			hi = ex + 1
		}
		if lo >= width || hi <= lo {
			continue
		}
		if hi > width {
			hi = width
		}
		left := ansi.Truncate(line, lo, "")
		mid := ansi.Strip(ansi.Cut(line, lo, hi))
		right := ansi.TruncateLeft(line, hi, "")
		lines[y] = left + invert.Render(mid) + right
	}
	return strings.Join(lines, "\n")
}

// paintToastOverlay lays the copied-toast into the status row's left
// padding (the status line is right-aligned, so its left side is dead
// space — the toast borrows it rather than shifting any layout). The ✓
// wears the OK green; the quip is the server's own (or absent — never a
// client-made line).
func (m Model) paintToastOverlay(body string) string {
	toast := lipgloss.NewStyle().Foreground(render.ColorOK).Render("✓")
	if m.toastText != "" {
		toast += " " + statusStyle.Render(m.toastText)
	}
	toastW := lipgloss.Width(toast)
	lines := strings.Split(body, "\n")
	y := len(lines) - 1 // the status row — viewContent always appends it last
	if y < 0 {
		return body
	}
	line := lines[y]
	if lipgloss.Width(line) < toastW+2 {
		return body // no dead space to borrow this frame — skip, never overlap
	}
	lines[y] = toast + ansi.TruncateLeft(line, toastW, "")
	return strings.Join(lines, "\n")
}
