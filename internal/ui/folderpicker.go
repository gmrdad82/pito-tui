// The local-filesystem folder navigator — a self-contained overlay in the
// same visual language as the show game / show vid picker (entitypicker.go:
// the pito-badge header, the right-edge Esc chip, the ─ rule, zebra rows
// with the ▌ cursor stripe, windowed scrolling with "↑/↓ N more"). Wired to
// ctrl+f's footage flow (footage.go), which owns a FolderPicker the same
// way Model owns entityPicker/aiPickerPanel. Built pure on purpose: no
// *Model dependency, no network, so it can be driven and asserted on in
// isolation.
//
// Contract (owner 2026-07-13):
//  1. Browses the local filesystem from a starting directory: subfolders
//     first, then video files (.mkv/.mp4/.mov/.avi, case-insensitive);
//     dotfiles hidden; enter descends into the highlighted folder,
//     backspace/left ascends to the parent.
//  2. space toggles the file under the cursor; a selects every video file
//     in the current folder; n clears every selection in the current
//     folder — all three are rendered as key hints in the footer.
//  3. Selections ACCUMULATE across folders for the life of the session
//     (keyed by absolute path, so navigating away and back never loses
//     them); the footer carries a running "N files selected · M folders"
//     tally. SelectedFiles reports the accumulated set.
//  4. The starting directory is a constructor argument; if it doesn't
//     exist, the picker falls back to the user's home directory.
//  5. Confirm/cancel mirror entitypicker's enter/esc exactly: esc cancels
//     unconditionally (matching the game picker's zero-value reset); enter
//     activates whatever is under the cursor — descend for a folder, or
//     (since a file can't be descended into) confirm the session for a
//     file, precisely how entitypicker's enter both is "activate the
//     highlighted row" and a no-op over an empty list.
//  6. Directory read errors (permission denied, races, etc.) render
//     inline, dim — never a panic.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

// videoExts is the folder navigator's file filter — case-insensitive,
// checked against filepath.Ext's lowercased result.
var videoExts = map[string]bool{
	".mkv": true,
	".mp4": true,
	".mov": true,
	".avi": true,
}

// folderEntry is one listed row: a subfolder or a video file, always
// carrying its absolute path (the identity SelectedFiles and the
// cross-folder selection map both key on).
type folderEntry struct {
	name  string
	path  string
	isDir bool
}

// FolderPicker is the navigator's pure state: no *Model, no I/O beyond
// the local filesystem, so it drives and asserts the same way whether or
// not anything has wired it into the app yet.
type FolderPicker struct {
	path      string // current absolute directory
	entries   []folderEntry
	cursor    int
	selected  map[string]bool // absolute file path -> selected; persists across navigation
	err       string          // last directory-read error, rendered inline dim
	confirmed bool
	canceled  bool
	width     int
	height    int
	truecolor bool
	// breadcrumb is an optional pre-rendered header line painted right
	// under the rule (contract below, footage.go's own persistent
	// "footage → <title> #<id>" line) — "" (the zero value) renders
	// nothing, so a picker no caller has annotated looks exactly like
	// before this existed.
	breadcrumb string
}

// NewFolderPicker opens the navigator rooted at start. When start no
// longer exists it falls back to the user's home directory (contract #4);
// any other failure (permission, not-a-directory, home dir unavailable
// too) surfaces as the picker's own inline dim error instead of a panic —
// reload() is where that happens.
func NewFolderPicker(start string) FolderPicker {
	root := start
	if _, err := os.Stat(root); err != nil && os.IsNotExist(err) {
		if home, herr := os.UserHomeDir(); herr == nil {
			root = home
		}
	}
	p := FolderPicker{
		path:     root,
		selected: map[string]bool{},
		width:    80,
		height:   24,
	}
	return p.reload()
}

// WithTruecolor sets the render tier (default: the 256-color fallback),
// mirroring Model's own WithTruecolor option — a fluent setter rather than
// a constructor option since the picker's only required constructor
// argument is the start directory (contract #4).
func (p FolderPicker) WithTruecolor(on bool) FolderPicker {
	p.truecolor = on
	return p
}

// WithBreadcrumb sets the picker's pre-rendered header line — a fluent
// setter for the same reason WithTruecolor is one: the picker's only
// required constructor argument is the start directory, everything else
// layers on after (contract #4). The caller renders the line itself (owner
// order 2026-07-13, footage.go's footageBreadcrumb): this package stays
// ignorant of what a "game" is, it just paints whatever string it's given.
func (p FolderPicker) WithBreadcrumb(line string) FolderPicker {
	p.breadcrumb = line
	return p
}

// Confirmed reports whether enter activated a file row — the session is
// over; a future caller reads SelectedFiles and closes the overlay.
func (p FolderPicker) Confirmed() bool { return p.confirmed }

// Canceled reports whether esc closed the overlay (entitypicker's
// zero-value-reset cancel, mirrored here as a terminal flag since this
// picker owns no Model mode to reset).
func (p FolderPicker) Canceled() bool { return p.canceled }

// CurrentPath returns the directory the picker is browsing right now — the
// single folder a caller persists as "last used" on confirm (contract #3 of
// the ctrl+f footage flow, footage.go). Deliberately NOT derived from
// SelectedFiles: the accumulated selection can span multiple folders, but
// there is exactly one folder open at the moment enter confirmed it.
func (p FolderPicker) CurrentPath() string { return p.path }

// SelectedFiles returns the accumulated selection as absolute paths,
// sorted for a deterministic result.
func (p FolderPicker) SelectedFiles() []string {
	out := make([]string, 0, len(p.selected))
	for path := range p.selected {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// tally is the footer's "N files selected · M folders" pair — folders
// counts the DISTINCT directories the accumulated files live in, not
// folders visited.
func (p FolderPicker) tally() (files, folders int) {
	files = len(p.selected)
	dirs := make(map[string]bool, files)
	for path := range p.selected {
		dirs[filepath.Dir(path)] = true
	}
	return files, len(dirs)
}

// reload re-lists the current directory, resetting the cursor. A read
// failure clears the entries and sets err instead of propagating —
// contract #6, never a panic.
func (p FolderPicker) reload() FolderPicker {
	entries, err := listDir(p.path)
	p.cursor = 0
	if err != nil {
		p.entries, p.err = nil, err.Error()
		return p
	}
	p.entries, p.err = entries, ""
	return p
}

// listDir lists one directory: dotfiles hidden, subfolders first
// (case-insensitive name order), then video files (case-insensitive
// extension match, same order) — contract #1.
func listDir(dir string) ([]folderEntry, error) {
	infos, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var dirs, files []folderEntry
	for _, info := range infos {
		name := info.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		if info.IsDir() {
			dirs = append(dirs, folderEntry{name: name, path: full, isDir: true})
			continue
		}
		if videoExts[strings.ToLower(filepath.Ext(name))] {
			files = append(files, folderEntry{name: name, path: full})
		}
	}
	byName := func(rows []folderEntry) func(i, j int) bool {
		return func(i, j int) bool {
			return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
		}
	}
	sort.Slice(dirs, byName(dirs))
	sort.Slice(files, byName(files))
	return append(dirs, files...), nil
}

// ── update ───────────────────────────────────────────────────────────────

// Update is the picker's bubbletea entry point: tea.WindowSizeMsg sizes
// the view (View takes no width/height of its own — the same shape
// Model.onResize feeds m.width/m.height from), tea.KeyPressMsg drives
// navigation and selection. Everything else passes through untouched.
func (p FolderPicker) Update(msg tea.Msg) (FolderPicker, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width, p.height = msg.Width, msg.Height
		return p, nil
	case tea.KeyPressMsg:
		return p.onKey(msg), nil
	}
	return p, nil
}

// onKey handles one keypress. A confirmed or canceled picker is a
// terminal state — like entitypicker after esc/enter, it stops reacting
// so a caller mid-teardown can't reopen it by accident.
func (p FolderPicker) onKey(msg tea.KeyPressMsg) FolderPicker {
	if p.confirmed || p.canceled {
		return p
	}
	switch msg.String() {
	case "esc":
		p.canceled = true
		return p
	case "up", "ctrl+p":
		if p.cursor > 0 {
			p.cursor--
		}
		return p
	case "down", "ctrl+n":
		if p.cursor < len(p.entries)-1 {
			p.cursor++
		}
		return p
	case "enter":
		// Mirrors entitypicker's own enter: no-op over an empty list,
		// otherwise activate the highlighted row (contract #5).
		if len(p.entries) == 0 {
			return p
		}
		entry := p.entries[p.cursor]
		if entry.isDir {
			p.path = entry.path
			return p.reload()
		}
		p.confirmed = true
		return p
	case "backspace", "left":
		parent := filepath.Dir(p.path)
		if parent == p.path {
			return p // already at the filesystem root — nothing to ascend to
		}
		p.path = parent
		return p.reload()
	case "space":
		if len(p.entries) == 0 {
			return p
		}
		entry := p.entries[p.cursor]
		if entry.isDir {
			return p // only files are selectable — contract #2
		}
		if p.selected[entry.path] {
			delete(p.selected, entry.path)
		} else {
			p.selected[entry.path] = true
		}
		return p
	case "a":
		for _, entry := range p.entries {
			if !entry.isDir {
				p.selected[entry.path] = true
			}
		}
		return p
	case "n":
		for _, entry := range p.entries {
			if !entry.isDir {
				delete(p.selected, entry.path)
			}
		}
		return p
	}
	return p
}

// ── view ─────────────────────────────────────────────────────────────────

// folderMarkerStyle paints the selected-file checkmark — OK green, the
// house color for a confirmed/positive state (distinct from the cursor's
// accent stripe and the header's brand purple).
var folderMarkerStyle = lipgloss.NewStyle().Foreground(render.ColorOK)

// View renders the overlay in entitypicker's own chrome: brand header +
// right-edge Esc chip, a rule, the optional breadcrumb line (WithBreadcrumb
// above), the current path, the windowed folder/file rows (zebra dim, ▌
// cursor stripe, ✓ selection marker), the key-hint footer (space/a/n/enter/
// esc — contract #2 requires these as hints), and the running selection
// tally (contract #3).
func (p FolderPicker) View() string {
	width := p.width
	if width < 20 {
		width = 20
	}

	var b strings.Builder
	head := pickerBadgeStyle.Render("pito") + " " + render.Brand("folders", p.truecolor)
	esc := render.Kbd("Esc", p.truecolor)
	if pad := width - lipgloss.Width(head) - lipgloss.Width(esc) - 1; pad > 0 {
		head += strings.Repeat(" ", pad) + esc
	}
	b.WriteString(head + "\n")
	rule := min(width-2, 44)
	if rule < 4 {
		rule = 4
	}
	b.WriteString(pickerDimStyle.Render(strings.Repeat("─", rule)) + "\n")
	if p.breadcrumb != "" {
		b.WriteString(p.breadcrumb + "\n")
	}
	b.WriteString("› " + p.path + "\n")

	var lines []string
	for i, entry := range p.entries {
		zebra := zebraRowStyle(i)
		marker := "  "
		if !entry.isDir && p.selected[entry.path] {
			marker = folderMarkerStyle.Render("✓") + " "
		}
		name := entry.name
		if entry.isDir {
			name += "/"
		}
		label := zebra.Render(name)
		if i == p.cursor {
			label = pickerCursorStyle.Render(name)
		}
		line := " " + marker + label
		if i == p.cursor {
			line = cursorStripe(line, width)
		} else {
			line = " " + line
		}
		lines = append(lines, line)
	}
	if len(p.entries) == 0 && p.err == "" {
		lines = append(lines, pickerDimStyle.Render("  (empty)"))
	}
	if p.err != "" {
		// Contract #6: dim, not the red notifErrStyle other panels use —
		// a read failure here is routine (permissions, races), not an
		// alarm.
		lines = append(lines, pickerDimStyle.Render("  "+p.err))
	}

	// Window like the other overlays: chrome above (3, +1 more when a
	// breadcrumb line is set) + footer below (3).
	chrome := 7
	if p.breadcrumb != "" {
		chrome++
	}
	visible := p.height - chrome
	if visible < 3 {
		visible = 3
	}
	cursorLine := min(p.cursor, max(len(lines)-1, 0))
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
	if start > 0 {
		b.WriteString(pickerDimStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
	}
	for _, line := range lines[start:end] {
		b.WriteString(line + "\n")
	}
	if end < len(lines) {
		b.WriteString(pickerDimStyle.Render(fmt.Sprintf("  ↓ %d more", len(lines)-end)) + "\n")
	}

	footer := pickerKeyStyle.Render("space") + pickerDimStyle.Render(" toggle · ") +
		pickerKeyStyle.Render("a") + pickerDimStyle.Render(" all · ") +
		pickerKeyStyle.Render("n") + pickerDimStyle.Render(" none · ") +
		pickerKeyStyle.Render("enter") + pickerDimStyle.Render(" open/confirm · ") +
		pickerKeyStyle.Render("esc") + pickerDimStyle.Render(" cancel")
	b.WriteString(footer + "\n")

	files, folders := p.tally()
	b.WriteString(pickerDimStyle.Render(fmt.Sprintf("%d files selected · %d folders", files, folders)))
	return b.String()
}
