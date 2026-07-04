package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/gmrdad82/pito-tui/internal/api"
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
		rows = append(rows, pickerRow{uuid: r.UUID, title: r.Title, last: r.LastActivityAt, section: "recent"})
	}
	for _, r := range list.Older {
		rows = append(rows, pickerRow{uuid: r.UUID, title: r.Title, last: r.LastActivityAt, section: "older"})
	}
	return rows
}

var (
	pickerTitleStyle   = lipgloss.NewStyle().Bold(true)
	pickerSectionStyle = lipgloss.NewStyle().Faint(true)
	pickerCursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	pickerDimStyle     = lipgloss.NewStyle().Faint(true)
)

// pickerView renders the conversation picker. now anchors the relative
// timestamps so golden frames stay deterministic.
func pickerView(rows []pickerRow, cursor, width int, now time.Time) string {
	var b strings.Builder
	b.WriteString(pickerTitleStyle.Render("pito — conversations") + "\n\n")
	section := ""
	for i, row := range rows {
		if row.section != "" && row.section != section {
			section = row.section
			b.WriteString(pickerSectionStyle.Render("— "+section) + "\n")
		}
		marker := "  "
		if i == cursor {
			marker = pickerCursorStyle.Render("> ")
		}
		label := row.title
		if row.isNew {
			label = "+ " + label
		} else if label == "" {
			label = "(unnamed)"
		}
		line := marker + label
		if !row.last.IsZero() {
			line += pickerDimStyle.Render("  · " + relativeTime(row.last, now))
		}
		b.WriteString(lipgloss.NewStyle().MaxWidth(width).Render(line) + "\n")
	}
	b.WriteString("\n" + pickerDimStyle.Render("j/k move · enter open · ctrl-c quit"))
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
