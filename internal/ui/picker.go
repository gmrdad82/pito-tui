package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

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
	section string // "recent" / "older" — group header painted above the first row of each
}

func pickerRows(list *api.ResumeList) []pickerRow {
	rows := []pickerRow{{isNew: true, title: "start a new conversation"}}
	for _, r := range list.Recent {
		rows = append(rows, pickerRow{uuid: r.UUID, title: r.Label(), last: r.LastActivityAt, section: "recent"})
	}
	for _, r := range list.Older {
		rows = append(rows, pickerRow{uuid: r.UUID, title: r.Label(), last: r.LastActivityAt, section: "older"})
	}
	return rows
}

var (
	pickerBadgeStyle   = lipgloss.NewStyle().Background(render.ColorPrimary).Foreground(lipgloss.Color("231")).Bold(true).Padding(0, 1)
	pickerTitleStyle   = lipgloss.NewStyle().Bold(true)
	pickerSectionStyle = lipgloss.NewStyle().Foreground(render.ColorPrimary).Bold(true)
	pickerCursorStyle  = lipgloss.NewStyle().Foreground(render.ColorAccent).Bold(true)
	pickerNewStyle     = lipgloss.NewStyle().Foreground(render.ColorOK)
	pickerDimStyle     = lipgloss.NewStyle().Foreground(render.ColorDim)
	pickerKeyStyle     = lipgloss.NewStyle().Foreground(render.ColorDim).Bold(true)
)

// pickerView renders the conversation picker, windowed so long lists keep
// the cursor on screen (title and help stay fixed). now anchors the
// relative timestamps so golden frames stay deterministic.
func pickerView(rows []pickerRow, cursor, width, height int, now time.Time) string {
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
		if i == cursor {
			marker = pickerCursorStyle.Render("▌ ")
			cursorLine = len(lines)
			if !row.isNew {
				label = pickerCursorStyle.Render(row.title)
			}
		}
		line := marker + label
		if !row.last.IsZero() {
			line += pickerDimStyle.Render("  · " + relativeTime(row.last, now))
		}
		lines = append(lines, lipgloss.NewStyle().MaxWidth(width).Render(line))
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
	b.WriteString(pickerBadgeStyle.Render("pito") + " " + pickerTitleStyle.Render("conversations") + "\n\n")
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
