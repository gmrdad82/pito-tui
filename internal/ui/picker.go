package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// pickerRow is one selectable line in the conversation picker: the "new
// conversation" entry or one resume row.
type pickerRow struct {
	uuid    string
	title   string
	last    time.Time
	isNew   bool
	ai      bool   // conversation carries an ai-kind event — sparkle badge
	section string // "recent" / "older" — group header painted above the first row of each
}

func pickerRows(list *api.ResumeList) []pickerRow {
	rows := []pickerRow{{isNew: true, title: "start a new conversation"}}
	return append(rows, resumeListRows(list)...)
}

// resumeListRows converts one /resume.json page's recent/older rows to
// pickerRows, WITHOUT the "new conversation" sentinel — pickerRows (the
// picker's first page) prepends that; onResume's pagination follow-on
// (tui-needs ask 9a) appends this straight onto the rows already on
// screen instead. Pages past the first fold their flat `rows` key into
// Older on the wire (ResumeList.UnmarshalJSON, events.go) and rarely
// carry Recent at all, but both slices are read identically here either
// way, so a page that DID carry genuine recent rows still slots in
// correctly, continuing (or starting) the RECENT/OLDER section run
// pickerView paints from.
func resumeListRows(list *api.ResumeList) []pickerRow {
	var rows []pickerRow
	for _, r := range list.Recent {
		rows = append(rows, pickerRow{uuid: r.UUID, title: r.Label(), last: r.LastActivityAt, ai: r.AI, section: "recent"})
	}
	for _, r := range list.Older {
		rows = append(rows, pickerRow{uuid: r.UUID, title: r.Label(), last: r.LastActivityAt, ai: r.AI, section: "older"})
	}
	return rows
}

var (
	pickerBadgeStyle   = lipgloss.NewStyle().Background(render.ColorPrimary).Foreground(lipgloss.Color("231")).Bold(true).Padding(0, 1)
	pickerSectionStyle = lipgloss.NewStyle().Foreground(render.ColorPrimary).Bold(true)
	pickerCursorStyle  = lipgloss.NewStyle().Foreground(render.ColorAccent).Bold(true)

	pickerNewStyle = lipgloss.NewStyle().Foreground(render.ColorOK)
	pickerDimStyle = lipgloss.NewStyle().Foreground(render.ColorDim)
	pickerKeyStyle = lipgloss.NewStyle().Foreground(render.ColorDim).Bold(true)
)

// pickerView renders the conversation picker, windowed so long lists keep
// the cursor on screen (title and help stay fixed). now anchors the
// relative timestamps so golden frames stay deterministic. phase is the
// model's global shimmer phase, threaded through so ai-flagged rows'
// sparkle badge rides the same sweep as the rest of the house animation.
// fetching appends the loadingDots row below the list — the picker's
// pagination follow-on (tui-needs ask 9a) fetching its next page,
// mirroring notificationsPanelView's loader row. A server that never
// sends next_cursor never sets this, so its frames render byte-identical
// to before pagination existed.
func pickerView(rows []pickerRow, cursor, width, height int, now time.Time, truecolor bool, phase float64, fetching, canClose bool) string {
	// Build every list line first, remembering which line the cursor is on
	// (section headers interleave, so row index ≠ line index).
	var lines []string
	cursorLine := 0
	section := ""
	for i, row := range rows {
		if row.section != "" && row.section != section {
			section = row.section
			lines = append(lines, pickerSectionStyle.Render(strings.ToUpper(section)))
		}
		marker := "  "
		label := row.title
		if row.isNew {
			label = pickerNewStyle.Render("+ " + label)
		} else if label == "" {
			label = "(unnamed)"
		}
		selected := i == cursor
		if selected {
			marker = pickerCursorStyle.Render("▌ ")
			cursorLine = len(lines)
			if !row.isNew {
				label = pickerCursorStyle.Render(row.title)
			}
		}
		if row.ai {
			label += " " + aiBadge(phase, truecolor)
		}
		line := marker + label
		if !row.last.IsZero() {
			line += pickerDimStyle.Render("  · " + relativeTime(row.last, now))
		}
		line = lipgloss.NewStyle().MaxWidth(width).Render(line)
		if selected {
			// The selection wears a full-width elevated-gray highlight —
			// retinted off the table zebra's old plum (owner 2026-07-12
			// "align to Charm"): this is a SELECTION affordance, not
			// decoration, so it keeps a background, just a neutral one.
			if pad := width - 1 - lipgloss.Width(line); pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			line = lipgloss.NewStyle().Background(render.ColorElevated).Render(line)
		}
		lines = append(lines, line)
	}
	if fetching {
		lines = append(lines, loadingDots(phase, truecolor, width))
	}

	// Window the list to the space between the fixed title (2 lines) and
	// help (2 lines), keeping the cursor visible.
	visible := height - 4
	if visible < 3 {
		visible = 3
	}
	start := 0
	if len(lines) > visible {
		start = cursorLine - visible/2
		if start < 0 {
			start = 0
		}
		if start > len(lines)-visible {
			start = len(lines) - visible
		}
	}
	end := start + visible
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	rule := width - 2
	if rule > 44 {
		rule = 44
	}
	if rule < 4 {
		rule = 4
	}
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand("conversations", truecolor)
	if canClose {
		// Opened over a live conversation (/resume) — show the way back,
		// right-edge chip like every other modal (owner 2026-07-12).
		esc := render.Kbd("Esc", truecolor)
		if pad := width - lipgloss.Width(head) - lipgloss.Width(esc) - 1; pad > 0 {
			head += strings.Repeat(" ", pad) + esc
		}
	}
	b.WriteString(head + "\n")
	b.WriteString(pickerDimStyle.Render(strings.Repeat("─", rule)) + "\n")
	if start > 0 {
		b.WriteString(pickerDimStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
	}
	for _, line := range lines[start:end] {
		b.WriteString(line + "\n")
	}
	if end < len(lines) {
		b.WriteString(pickerDimStyle.Render(fmt.Sprintf("  ↓ %d more", len(lines)-end)) + "\n")
	}
	help := pickerKeyStyle.Render("j/k") + pickerDimStyle.Render(" move · ") +
		pickerKeyStyle.Render("enter") + pickerDimStyle.Render(" open · ") +
		pickerKeyStyle.Render("ctrl-c") + pickerDimStyle.Render(" quit")
	b.WriteString("\n" + help)
	return b.String()
}

func relativeTime(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// zebraRowStyle is the ls-vids table language for modal lists —
// alternating neutral-gray FOREGROUNDS for resting rows (owner
// 2026-07-13: "use what you used for ls vids. It's way cooler").
func zebraRowStyle(i int) lipgloss.Style {
	if i%2 == 0 {
		return lipgloss.NewStyle().Foreground(render.ColorDim)
	}
	return lipgloss.NewStyle().Foreground(render.ColorFaint)
}

// cursorStripe paints the SELECTED modal row: the ▌ accent bar plus the
// plum zebra stripe across the full row (owner 2026-07-13: "use the
// zebra effect from ls vids to show the cursor") — one cursor language
// for /resume, /notifications, ctrl+k, and the show game/vid pickers.
func cursorStripe(line string, width int) string {
	if pad := width - 1 - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return pickerCursorStyle.Render("▌") + lipgloss.NewStyle().Background(render.ColorZebra).Render(line)
}
