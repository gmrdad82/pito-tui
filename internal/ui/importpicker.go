// The import-game picker — the TUI face of the web's games-import sidebar
// (owner 2026-07-14: "pito tui doesn't support import game").
//
// The grammar forms that open the web's sidebar (`import`, `import game`,
// `import <title>`, `/games import [title]`) intercept client-side — the
// server would only answer them with browser-only sidebar chrome — and open
// this overlay instead. Typing searches IGDB through POST /games/search:
// the codebase's FIRST remote as-you-type search, debounced (300ms tick
// keyed to a generation counter so stale replies and stale ticks both
// drop). Hits already in the library are labeled — picking one re-syncs,
// the Importer's own semantics. Enter POSTs /games/import and returns to
// chat: the import narrates itself into the scrollback (announce → done)
// over the cable, so the picker's goodbye is just a notice. `import
// videos`/`vids` stays untouched — that's the sync-videos alias, not ours.
package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// importDebounce is how long typing must rest before a search fires —
// matches the web overlay's 250ms plus margin for terminal key repeat.
const importDebounce = 300 * time.Millisecond

type importPicker struct {
	hits      []api.IgdbHit
	library   map[int]bool
	cursor    int
	query     string
	gen       int // debounce + stale-reply generation: bumped on every edit
	searching bool
	searched  bool // one search has completed (drives the empty-state line)
	err       string
}

// ImportSearchTickMsg is the debounce alarm: only the tick matching the
// CURRENT generation fires a search.
type ImportSearchTickMsg struct{ Gen int }

// ImportSearchedMsg carries one /games/search answer, generation-tagged.
type ImportSearchedMsg struct {
	Gen int
	Res *api.IgdbSearch
	Err error
}

// GameImportedMsg is the /games/import outcome (title bound at dispatch,
// the aipicker outcome-flash pattern).
type GameImportedMsg struct {
	Title string
	Err   error
}

// importTrigger recognizes the sidebar-opening grammar forms. Returns the
// title prefill (possibly "") and whether the text is ours. The videos
// spellings belong to the sync alias and are deliberately NOT matched.
func importTrigger(text string) (prefill string, ok bool) {
	t := strings.TrimSpace(text)
	lower := strings.ToLower(t)
	switch {
	case lower == "import", lower == "import game", lower == "import games", lower == "/games import":
		return "", true
	case strings.HasPrefix(lower, "/games import "):
		return strings.TrimSpace(t[len("/games import "):]), true
	case strings.HasPrefix(lower, "import game "):
		return strings.TrimSpace(t[len("import game "):]), true
	case strings.HasPrefix(lower, "import games "):
		return strings.TrimSpace(t[len("import games "):]), true
	case strings.HasPrefix(lower, "import "):
		rest := strings.TrimSpace(t[len("import "):])
		switch strings.ToLower(rest) {
		case "video", "videos", "vid", "vids":
			return "", false
		}
		return rest, true
	}
	return "", false
}

// openImportPicker opens the overlay; a prefill searches immediately (no
// debounce — the owner already finished typing that query in the chatbox).
func (m Model) openImportPicker(prefill string) (tea.Model, tea.Cmd) {
	m.mode = modeImport
	m.importP = importPicker{query: prefill, library: map[int]bool{}}
	if prefill == "" {
		return m, nil
	}
	m.importP.gen++
	m.importP.searching = true
	return m, tea.Batch(m.importSearchCmd(prefill, m.importP.gen), m.animate())
}

func (m Model) importSearchCmd(query string, gen int) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		res, err := client.SearchIGDB(context.Background(), query)
		return ImportSearchedMsg{Gen: gen, Res: res, Err: err}
	}
}

func importDebounceCmd(gen int) tea.Cmd {
	return tea.Tick(importDebounce, func(time.Time) tea.Msg {
		return ImportSearchTickMsg{Gen: gen}
	})
}

func (m Model) onImportSearchTick(msg ImportSearchTickMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeImport || msg.Gen != m.importP.gen || m.importP.query == "" {
		return m, nil // superseded by more typing, or the picker closed
	}
	m.importP.searching = true
	return m, tea.Batch(m.importSearchCmd(m.importP.query, m.importP.gen), m.animate())
}

func (m Model) onImportSearched(msg ImportSearchedMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeImport || msg.Gen != m.importP.gen {
		return m, nil // a newer query owns the panel now
	}
	m.importP.searching = false
	m.importP.searched = true
	if msg.Err != nil {
		if isUnauthorized(msg.Err) {
			m.needsLogin = true
		}
		m.importP.err = msg.Err.Error()
		return m, nil
	}
	m.importP.err = msg.Res.ErrorMessage
	m.importP.hits = msg.Res.Hits
	m.importP.library = msg.Res.LibraryIDs
	m.importP.cursor = 0
	return m, nil
}

func (m Model) onGameImported(msg GameImportedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		if isUnauthorized(msg.Err) {
			m.needsLogin = true
		}
		m.pushNotice("import failed: " + msg.Err.Error())
		return m, nil
	}
	m.pushNotice("importing " + msg.Title + " — the chat will narrate it")
	return m, nil
}

func (m Model) onImportKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeChat
		m.importP = importPicker{}
		return m, nil
	case "up", "ctrl+p":
		if m.importP.cursor > 0 {
			m.importP.cursor--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.importP.cursor < len(m.importP.hits)-1 {
			m.importP.cursor++
		}
		return m, nil
	case "enter":
		if len(m.importP.hits) == 0 {
			return m, nil
		}
		hit := m.importP.hits[min(m.importP.cursor, len(m.importP.hits)-1)]
		client, uuid := m.client, m.conv.UUID
		m.mode = modeChat
		m.importP = importPicker{}
		m.sounds.Send()
		return m, func() tea.Msg {
			err := client.ImportGame(context.Background(), hit.ID, hit.Name, uuid)
			return GameImportedMsg{Title: hit.Name, Err: err}
		}
	case "backspace":
		if m.importP.query != "" {
			runes := []rune(m.importP.query)
			m.importP.query = string(runes[:len(runes)-1])
			m.importP.cursor = 0
			m.importP.gen++
			if m.importP.query == "" {
				m.importP.hits = nil
				m.importP.searching = false
				return m, nil
			}
			return m, importDebounceCmd(m.importP.gen)
		}
		return m, nil
	}
	if text := msg.Key().Text; text != "" {
		m.importP.query += text
		m.importP.cursor = 0
		m.importP.gen++
		return m, importDebounceCmd(m.importP.gen)
	}
	return m, nil
}

// importHintStyle mirrors footageHintStyle: ColorCyan, the house
// "informational, worth noticing" hue.
var importHintStyle = lipgloss.NewStyle().Foreground(render.ColorCyan)

func (m Model) importPickerView() string {
	p := m.importP
	width := m.contentWidth()

	var b strings.Builder
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand("import", m.truecolor)
	esc := render.Kbd("Esc", m.truecolor)
	if pad := width - lipgloss.Width(head) - lipgloss.Width(esc) - 1; pad > 0 {
		head += strings.Repeat(" ", pad) + esc
	}
	b.WriteString(head + "\n")
	rule := min(width-2, 44)
	if rule < 4 {
		rule = 4
	}
	b.WriteString(pickerDimStyle.Render(strings.Repeat("─", rule)) + "\n")
	b.WriteString(importHintStyle.Render("import a game — type to search IGDB") + "\n")
	if p.query == "" {
		b.WriteString("› " + pickerDimStyle.Render(render.PitoCopy.Palette.SearchPlaceholder) + "\n")
	} else {
		b.WriteString("› " + p.query + "\n")
	}

	var lines []string
	for i, hit := range p.hits {
		label := hit.Name
		if hit.TypeNote != "" {
			label += " " + hit.TypeNote
		}
		right := strconv.Itoa(hit.ID)
		if p.library[hit.ID] {
			right = "in library · " + right
		}
		zebra := zebraRowStyle(i)
		title := zebra.Render(label)
		if i == p.cursor {
			title = pickerCursorStyle.Render(label)
		}
		line := " " + title
		if pad := width - 1 - lipgloss.Width(line) - lipgloss.Width(right) - 2; pad > 0 {
			line += strings.Repeat(" ", pad) + zebra.Render(right) + "  "
		}
		if i == p.cursor {
			line = cursorStripe(line, width)
		} else {
			line = " " + line
		}
		lines = append(lines, line)
	}
	if p.searching {
		lines = append(lines, loadingDots(m.phase, m.truecolor, width))
	}
	if p.err != "" {
		lines = append(lines, notifErrStyle.Render("  "+p.err))
	}
	if !p.searching && p.err == "" && len(p.hits) == 0 && p.searched && p.query != "" {
		lines = append(lines, pickerDimStyle.Render("  nothing on IGDB by that name"))
	}

	chrome := 6 // title + rule + hint + query + help margins
	visible := m.height - chrome
	if visible < 3 {
		visible = 3
	}
	cursorLine := min(p.cursor, len(lines)-1)
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
	end := min(start+visible, len(lines))
	if end < 0 {
		end = 0
	}
	if start > 0 {
		b.WriteString(pickerDimStyle.Render(strings.Repeat(" ", 2)+"↑ more") + "\n")
	}
	for _, line := range lines[start:end] {
		b.WriteString(line + "\n")
	}
	if end < len(lines) {
		b.WriteString(pickerDimStyle.Render("  ↓ more") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
