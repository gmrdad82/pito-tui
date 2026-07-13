package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// folderPickerFixture builds a small tree under t.TempDir():
//
//	root/
//	  Docs/                 (empty folder)
//	  Videos/
//	    Sub/
//	      clip3.mov
//	      readme.md          (non-video — filtered)
//	    .hidden.mp4          (dotfile — hidden)
//	    clip1.mp4
//	    clip2.MKV             (case-insensitive extension match)
//	    notes.txt             (non-video — filtered)
//	  .hidden_dir/            (dotfile folder — hidden)
//	  movie.avi               (top-level video)
//	  plain.txt               (top-level non-video — filtered)
//
// Returns the root and the individual paths the tests assert against.
func folderPickerFixture(t *testing.T) (root string, videosDir, subDir string) {
	t.Helper()
	root = t.TempDir()
	videosDir = filepath.Join(root, "Videos")
	subDir = filepath.Join(videosDir, "Sub")
	dirs := []string{
		filepath.Join(root, "Docs"),
		videosDir,
		subDir,
		filepath.Join(root, ".hidden_dir"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	files := map[string]string{
		filepath.Join(subDir, "clip3.mov"):      "x",
		filepath.Join(subDir, "readme.md"):      "x",
		filepath.Join(videosDir, ".hidden.mp4"): "x",
		filepath.Join(videosDir, "clip1.mp4"):   "x",
		filepath.Join(videosDir, "clip2.MKV"):   "x",
		filepath.Join(videosDir, "notes.txt"):   "x",
		filepath.Join(root, "movie.avi"):        "x",
		filepath.Join(root, "plain.txt"):        "x",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root, videosDir, subDir
}

func pressKey(p FolderPicker, msg tea.KeyPressMsg) FolderPicker {
	next, _ := p.Update(msg)
	return next
}

var (
	keyUp        = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown      = tea.KeyPressMsg{Code: tea.KeyDown}
	keyEnter     = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc       = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyBackspace = tea.KeyPressMsg{Code: tea.KeyBackspace}
	keyLeft      = tea.KeyPressMsg{Code: tea.KeyLeft}
	keySpace     = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	keyAll       = tea.KeyPressMsg{Code: 'a', Text: "a"}
	keyNone      = tea.KeyPressMsg{Code: 'n', Text: "n"}
)

// entryNames extracts the visible listing (folders carry a trailing "/",
// matching View's own rendering) so tests can assert order and filtering
// without reaching into folderEntry directly.
func entryNames(p FolderPicker) []string {
	names := make([]string, len(p.entries))
	for i, e := range p.entries {
		if e.isDir {
			names[i] = e.name + "/"
		} else {
			names[i] = e.name
		}
	}
	return names
}

func TestFolderPickerListingFiltersAndOrders(t *testing.T) {
	root, videosDir, subDir := folderPickerFixture(t)

	tests := []struct {
		name string
		dir  string
		want []string
	}{
		{"root: dotfiles hidden, folders before files, non-video filtered", root, []string{"Docs/", "Videos/", "movie.avi"}},
		{"Videos: case-insensitive video match, dotfile+non-video filtered", videosDir, []string{"Sub/", "clip1.mp4", "clip2.MKV"}},
		{"Sub: only the video file survives", subDir, []string{"clip3.mov"}},
		{"Docs: empty directory lists nothing", filepath.Join(root, "Docs"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewFolderPicker(tc.dir)
			got := entryNames(p)
			if len(got) != len(tc.want) {
				t.Fatalf("entries = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("entries = %v, want %v", got, tc.want)
				}
			}
			if p.err != "" {
				t.Fatalf("unexpected err: %q", p.err)
			}
		})
	}
}

func TestFolderPickerNavigation(t *testing.T) {
	root, videosDir, subDir := folderPickerFixture(t)
	p := NewFolderPicker(root)
	if p.path != root {
		t.Fatalf("path = %q, want %q", p.path, root)
	}

	// root listing is [Docs/, Videos/, movie.avi] — move to Videos/ and
	// descend.
	p = pressKey(p, keyDown)
	p = pressKey(p, keyEnter)
	if p.path != videosDir {
		t.Fatalf("after enter on Videos/, path = %q, want %q", p.path, videosDir)
	}
	if p.cursor != 0 {
		t.Fatalf("cursor must reset on descend, got %d", p.cursor)
	}

	// Videos/ listing is [Sub/, clip1.mp4, clip2.MKV] — descend into Sub/.
	p = pressKey(p, keyEnter)
	if p.path != subDir {
		t.Fatalf("after enter on Sub/, path = %q, want %q", p.path, subDir)
	}

	// backspace ascends one level at a time.
	p = pressKey(p, keyBackspace)
	if p.path != videosDir {
		t.Fatalf("after backspace, path = %q, want %q", p.path, videosDir)
	}
	p = pressKey(p, keyLeft)
	if p.path != root {
		t.Fatalf("after left, path = %q, want %q", p.path, root)
	}

	// Ascending past the filesystem root is a no-op, never a panic.
	fsRoot := filepath.Dir(root)
	for fsRoot != filepath.Dir(fsRoot) {
		fsRoot = filepath.Dir(fsRoot)
	}
	atRoot := NewFolderPicker(fsRoot)
	atRoot = pressKey(atRoot, keyBackspace)
	if atRoot.path != fsRoot {
		t.Fatalf("ascending past the filesystem root moved: %q, want %q", atRoot.path, fsRoot)
	}
}

func TestFolderPickerToggleAllNone(t *testing.T) {
	_, videosDir, _ := folderPickerFixture(t)
	p := NewFolderPicker(videosDir)
	// entries: [Sub/, clip1.mp4, clip2.MKV]

	// space on the folder row (cursor 0) is a no-op — only files select.
	p = pressKey(p, keySpace)
	if len(p.selected) != 0 {
		t.Fatalf("space on a folder must not select anything, got %v", p.selected)
	}

	// space on clip1.mp4 toggles it on, then off.
	p = pressKey(p, keyDown)
	p = pressKey(p, keySpace)
	clip1 := filepath.Join(videosDir, "clip1.mp4")
	if !p.selected[clip1] {
		t.Fatalf("clip1.mp4 must be selected after space")
	}
	p = pressKey(p, keySpace)
	if p.selected[clip1] {
		t.Fatalf("clip1.mp4 must be deselected after a second space")
	}

	// 'a' selects every video file in the folder (not the subfolder).
	p = pressKey(p, keyAll)
	clip2 := filepath.Join(videosDir, "clip2.MKV")
	if !p.selected[clip1] || !p.selected[clip2] {
		t.Fatalf("'a' must select every file in the folder, got %v", p.selected)
	}
	if files, folders := p.tally(); files != 2 || folders != 1 {
		t.Fatalf("tally = %d files / %d folders, want 2/1", files, folders)
	}

	// 'n' clears every selection in the folder.
	p = pressKey(p, keyNone)
	if len(p.selected) != 0 {
		t.Fatalf("'n' must clear the folder's selections, got %v", p.selected)
	}
}

func TestFolderPickerAccumulatesAcrossFolders(t *testing.T) {
	root, videosDir, subDir := folderPickerFixture(t)
	p := NewFolderPicker(root)

	// Descend to Videos/, select clip2.MKV (cursor: Sub/, clip1.mp4, clip2.MKV).
	p = pressKey(p, keyDown) // root cursor -> Videos/
	p = pressKey(p, keyEnter)
	p = pressKey(p, keyDown) // -> clip1.mp4
	p = pressKey(p, keyDown) // -> clip2.MKV
	p = pressKey(p, keySpace)
	clip2 := filepath.Join(videosDir, "clip2.MKV")
	if !p.selected[clip2] {
		t.Fatalf("clip2.MKV must be selected")
	}

	// Descend into Sub/ and select clip3.mov.
	p = pressKey(p, keyUp)
	p = pressKey(p, keyUp) // back to Sub/ (cursor 0)
	p = pressKey(p, keyEnter)
	p = pressKey(p, keySpace)
	clip3 := filepath.Join(subDir, "clip3.mov")
	if !p.selected[clip3] {
		t.Fatalf("clip3.mov must be selected")
	}

	// Navigate all the way back to Videos/ — clip2.MKV's selection must
	// still be there (the accumulation contract), and the running tally
	// must count 2 files across 2 distinct folders.
	p = pressKey(p, keyBackspace)
	if !p.selected[clip2] || !p.selected[clip3] {
		t.Fatalf("selections must persist across navigation, got %v", p.selected)
	}
	if files, folders := p.tally(); files != 2 || folders != 2 {
		t.Fatalf("tally = %d files / %d folders, want 2/2", files, folders)
	}

	got := p.SelectedFiles()
	want := []string{clip2, clip3}
	sort.Strings(want)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("SelectedFiles = %v, want %v", got, want)
	}
}

func TestFolderPickerMissingStartDirFallsBackToHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory available in this environment")
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	p := NewFolderPicker(missing)
	if p.path != home {
		t.Fatalf("path = %q, want the home directory %q", p.path, home)
	}
}

func TestFolderPickerConfirmDoesNotImplicitlyToggle(t *testing.T) {
	_, videosDir, _ := folderPickerFixture(t)
	p := NewFolderPicker(videosDir)
	// entries: [Sub/, clip1.mp4, clip2.MKV]. Select clip2.MKV explicitly,
	// then confirm with the cursor resting on clip1.mp4 — confirm must
	// return exactly what was toggled, never the row enter landed on.
	p = pressKey(p, keyDown)
	p = pressKey(p, keyDown)
	p = pressKey(p, keySpace) // select clip2.MKV
	p = pressKey(p, keyUp)    // cursor -> clip1.mp4 (never toggled)

	p = pressKey(p, keyEnter)
	if !p.confirmed {
		t.Fatal("enter on a file row must confirm the session")
	}
	if p.canceled {
		t.Fatal("confirm must not also cancel")
	}
	clip1 := filepath.Join(videosDir, "clip1.mp4")
	clip2 := filepath.Join(videosDir, "clip2.MKV")
	got := p.SelectedFiles()
	if len(got) != 1 || got[0] != clip2 {
		t.Fatalf("SelectedFiles = %v, want only %v (clip1 %v must stay unselected)", got, clip2, clip1)
	}

	// A confirmed picker is terminal — further keys are no-ops.
	before := p.cursor
	p = pressKey(p, keyDown)
	if p.cursor != before {
		t.Fatal("a confirmed picker must ignore further keys")
	}
}

func TestFolderPickerEnterOnEmptyDirIsNoOp(t *testing.T) {
	root, _, _ := folderPickerFixture(t)
	p := NewFolderPicker(filepath.Join(root, "Docs"))
	if len(p.entries) != 0 {
		t.Fatalf("Docs/ must list empty, got %v", entryNames(p))
	}
	p = pressKey(p, keyEnter)
	if p.confirmed || p.canceled {
		t.Fatal("enter over an empty directory must no-op, matching entitypicker's empty-list guard")
	}
}

func TestFolderPickerCancel(t *testing.T) {
	root, _, _ := folderPickerFixture(t)
	p := NewFolderPicker(root)
	p = pressKey(p, keyDown)
	p = pressKey(p, keyEnter) // descend into Videos/
	p = pressKey(p, keyEsc)
	if !p.canceled {
		t.Fatal("esc must cancel")
	}
	if p.confirmed {
		t.Fatal("cancel must not also confirm")
	}

	// A canceled picker is terminal too.
	before := p.path
	p = pressKey(p, keyBackspace)
	if p.path != before {
		t.Fatal("a canceled picker must ignore further keys")
	}
}

func TestFolderPickerReadErrorRendersInlineWithoutPanic(t *testing.T) {
	root, _, _ := folderPickerFixture(t)
	// Point the picker at a regular file — os.ReadDir fails deterministically
	// regardless of OS/permission bits, unlike chmod-based fixtures.
	p := NewFolderPicker(filepath.Join(root, "movie.avi"))
	if p.err == "" {
		t.Fatal("reading a non-directory must set err")
	}
	if len(p.entries) != 0 {
		t.Fatalf("a failed listing must leave entries empty, got %v", p.entries)
	}

	view := ansi.Strip(p.View())
	if view == "" {
		t.Fatal("View must still render something on a read error")
	}
}

func TestFolderPickerViewShowsHintsAndTally(t *testing.T) {
	_, videosDir, _ := folderPickerFixture(t)
	p := NewFolderPicker(videosDir)
	p = pressKey(p, keyDown)
	p = pressKey(p, keySpace) // select clip1.mp4

	view := ansi.Strip(p.View())
	for _, want := range []string{"space", "toggle", "a", "all", "n", "none", "enter", "cancel", "1 files selected · 1 folders"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}
