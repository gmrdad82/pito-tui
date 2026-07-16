// The ctrl+k command palette — owner smoke 2026-07-12: "we're missing
// ctrl+k for commands similar to pito web."
//
// A faithful port of the web pair: the item STRUCTURE mirrors pito's
// lib/pito/palette/command_catalog.rb (sections and inserts are code
// there, code here — the same documented-port precedent as analyze.go's
// barPresentation), the WORDS come from pito's palette locale via
// copygen (COPY LAW), and the behavior mirrors
// command_palette_controller.js: Ctrl+K toggles, Esc closes, ↑↓ walk the
// filtered list, Enter pre-fills the prompt with the item's insert text
// (placeholders included) and does NOT submit. The filter is the same
// fuzzy subsequence match — every query character must appear in the
// label, in order, case-insensitive; an empty query shows everything.
// Auth gating mirrors the catalog's: an unauthenticated session sees the
// single /login command.
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// ctrlKItem mirrors one CommandCatalog item: the locale key's last
// segment (resolved against the copygen mirror) and the pre-fill text.
// (The web item shape also carries an optional right-hand `shortcut`
// hint — today's catalog defines none, so the field waits in pito until
// one exists.)
type ctrlKItem struct {
	labelKey string
	insert   string
}

type ctrlKSection struct {
	titleKey string
	items    []ctrlKItem
}

// ctrlKCatalog is CommandCatalog#sections(authenticated:), verbatim.
func ctrlKCatalog(authenticated bool) []ctrlKSection {
	if !authenticated {
		return []ctrlKSection{{titleKey: "general", items: []ctrlKItem{
			{labelKey: "login", insert: "/login <code>"},
		}}}
	}
	return []ctrlKSection{
		{titleKey: "youtube", items: []ctrlKItem{
			{labelKey: "connect", insert: "/connect"},
			{labelKey: "disconnect", insert: "/disconnect <@handle>"},
			{labelKey: "import_game", insert: "import game"},
			// search_games_for / search_games_like: label keys just landed
			// in pito's palette locale — until the copygen mirror is
			// re-pinned against pito 3.0.0 (queued final task), they're
			// absent from render.PitoCopy and degrade to the insert text
			// (ctrlKLabel's COPY LAW fallback). Deliberate, not a bug.
			{labelKey: "search_games_for", insert: "search games for "},
			{labelKey: "search_games_like", insert: "search games like "},
		}},
		{titleKey: "config", items: []ctrlKItem{
			{labelKey: "config_ai", insert: "/config ai"},
			{labelKey: "config_google", insert: "/config google"},
			{labelKey: "config_igdb", insert: "/config igdb"},
			{labelKey: "config_webhook", insert: "/config webhook"},
		}},
		{titleKey: "conversations", items: []ctrlKItem{
			{labelKey: "new", insert: "/new"},
			{labelKey: "resume", insert: "/resume"},
			// search_conversations_for / search_conversations_like: same
			// pending copygen re-pin as the youtube section's search_games_*
			// pair above — see that comment.
			{labelKey: "search_conversations_for", insert: "search conversations for "},
			{labelKey: "search_conversations_like", insert: "search conversations like "},
		}},
		{titleKey: "general", items: []ctrlKItem{
			{labelKey: "help", insert: "/help"},
			{labelKey: "logout", insert: "/logout"},
		}},
	}
}

func ctrlKLabel(item ctrlKItem) string {
	if label := render.PitoCopy.Palette.Commands[item.labelKey]; label != "" {
		return label
	}
	// COPY LAW degrade: no mirrored label (older pito ref) → the insert
	// text itself stands in — it is server grammar, not client prose.
	return item.insert
}

// ctrlKFuzzy is the controller's #fuzzy: subsequence match, case-
// insensitive, empty query matches everything.
func ctrlKFuzzy(query, label string) bool {
	query, label = strings.ToLower(query), strings.ToLower(label)
	i := 0
	for _, ru := range label {
		if i < len(query) && rune(query[i]) == ru {
			i++
		}
	}
	return i == len(query)
}

// ctrlKPanel is the overlay's state: the live query and the cursor
// position within the FILTERED flat item list.
type ctrlKPanel struct {
	query string
	sel   int
}

// visibleItems flattens the auth-gated catalog through the fuzzy filter.
func (m Model) ctrlKVisibleItems() []ctrlKItem {
	var out []ctrlKItem
	for _, sec := range ctrlKCatalog(!m.needsLogin) {
		for _, item := range sec.items {
			if ctrlKFuzzy(m.ctrlK.query, ctrlKLabel(item)) {
				out = append(out, item)
			}
		}
	}
	return out
}

// openCtrlK opens the palette overlay with a fresh panel + slide-up.
func (m Model) openCtrlK() (tea.Model, tea.Cmd) {
	m.mode = modeCommandPalette
	m.ctrlK = ctrlKPanel{}
	return m, m.animate()
}

func (m Model) onCtrlKKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+k":
		m.mode = modeChat
		m.ctrlK = ctrlKPanel{}
		return m, nil
	case "up", "ctrl+p":
		if m.ctrlK.sel > 0 {
			m.ctrlK.sel--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.ctrlK.sel < len(m.ctrlKVisibleItems())-1 {
			m.ctrlK.sel++
		}
		return m, nil
	case "enter":
		items := m.ctrlKVisibleItems()
		if len(items) == 0 {
			return m, nil
		}
		sel := min(m.ctrlK.sel, len(items)-1)
		// Pre-fill only — the user fills <placeholders> and submits
		// themselves (command_palette_controller.js's Enter contract).
		m.input.SetValue(items[sel].insert)
		m.input.CursorEnd()
		m.suggest = nil
		m.mode = modeChat
		m.ctrlK = ctrlKPanel{}
		return m, nil
	case "backspace":
		if m.ctrlK.query != "" {
			runes := []rune(m.ctrlK.query)
			m.ctrlK.query = string(runes[:len(runes)-1])
			m.ctrlK.sel = 0
		}
		return m, nil
	}
	// Printable characters type into the search query.
	if text := msg.Key().Text; text != "" {
		m.ctrlK.query += text
		m.ctrlK.sel = 0
	}
	return m, nil
}

// ctrlKView renders the palette overlay: title + esc chip, the query
// line, then the sections with the selection riding the filtered flat
// index. Every word on screen is pito's own (palette locale via copygen).
func (m Model) ctrlKView() string {
	pal := render.PitoCopy.Palette
	width := m.contentWidth()

	var b strings.Builder
	title := pal.Title
	if title == "" {
		title = "ctrl+k" // COPY LAW degrade: the keybinding itself, not prose
	}
	// The Esc chip sits at the modal's RIGHT edge (owner 2026-07-12,
	// screenshot), not beside the title.
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand(title, m.truecolor)
	esc := render.Kbd(pal.EscHint, m.truecolor)
	if pad := width - lipgloss.Width(head) - lipgloss.Width(esc) - 1; pad > 0 {
		head += strings.Repeat(" ", pad) + esc
	} else {
		head += "  " + esc
	}
	b.WriteString(head + "\n")
	rule := min(width-2, 44)
	if rule < 4 {
		rule = 4
	}
	b.WriteString(pickerDimStyle.Render(strings.Repeat("─", rule)) + "\n")

	// The query line — placeholder dim until something is typed.
	if m.ctrlK.query == "" {
		b.WriteString("› " + pickerDimStyle.Render(pal.SearchPlaceholder) + "\n\n")
	} else {
		b.WriteString("› " + m.ctrlK.query + "\n\n")
	}

	items := m.ctrlKVisibleItems()
	sel := min(m.ctrlK.sel, max(len(items)-1, 0))
	flat := 0
	for _, sec := range ctrlKCatalog(!m.needsLogin) {
		var rows []string
		for _, item := range sec.items {
			if !ctrlKFuzzy(m.ctrlK.query, ctrlKLabel(item)) {
				continue
			}
			label := ctrlKLabel(item)
			insert := item.insert
			// ls-vids language: fg zebra + ▌ cursor (owner 2026-07-13).
			zebra := zebraRowStyle(flat)
			painted := zebra.Render(label)
			if flat == sel {
				painted = pickerCursorStyle.Render(label)
			}
			line := " " + painted
			if pad := width - 1 - lipgloss.Width(line) - lipgloss.Width(insert) - 2; pad > 0 {
				line += strings.Repeat(" ", pad) + zebra.Render(insert) + "  "
			}
			if flat == sel {
				line = cursorStripe(line, width)
			} else {
				line = " " + line
			}
			rows = append(rows, line)
			flat++
		}
		if len(rows) == 0 {
			continue // section fully filtered out — title hides too (web's #syncSectionVisibility)
		}
		sectionTitle := pal.Sections[sec.titleKey]
		if sectionTitle != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(render.ColorPrimary).Bold(true).Render(sectionTitle) + "\n")
		}
		b.WriteString(strings.Join(rows, "\n") + "\n\n")
	}
	if len(items) == 0 {
		b.WriteString(pickerDimStyle.Render(fmt.Sprintf("  ~ %q", m.ctrlK.query)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
