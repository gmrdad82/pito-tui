package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// fakeFootageExec seams footageExec for tests: LookPath answers
// lookPathErr (nil ⇒ found), Output answers a configured duration (in
// seconds) for a file, or an error for a file listed in unreadable — the
// contract's "unreadable files count 0 / tallied as skipped" path.
type fakeFootageExec struct {
	lookPathErr error
	durations   map[string]float64
	unreadable  map[string]bool
}

func (f *fakeFootageExec) LookPath(name string) (string, error) {
	if f.lookPathErr != nil {
		return "", f.lookPathErr
	}
	return "/usr/bin/" + name, nil
}

func (f *fakeFootageExec) Output(_ context.Context, _ string, args ...string) ([]byte, error) {
	file := args[len(args)-1]
	if f.unreadable[file] {
		return nil, errors.New("fake ffprobe: unreadable")
	}
	d, ok := f.durations[file]
	if !ok {
		return nil, fmt.Errorf("fake ffprobe: no duration configured for %s", file)
	}
	return []byte(strconv.FormatFloat(d, 'f', -1, 64)), nil
}

// footageServer serves one page of /games/picker.json (a single row) and a
// /chat sink recording every sent input — the game-picker fetch and the
// flow's final send both need a live handler.
func footageServer(t *testing.T, sent chan string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/games/picker.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rows":        []map[string]any{{"id": 42, "title": "Elden Ring"}},
			"next_cursor": nil,
		})
	})
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent <- body.Input
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"turn_id":7}`)
	})
	return mux
}

// ── gate ────────────────────────────────────────────────────────────────

func TestFootageGateMissingFfprobeOpensWarningNotPicker(t *testing.T) {
	fake := &fakeFootageExec{lookPathErr: errors.New("exec: \"ffprobe\": executable file not found in $PATH")}
	m, _ := newTestModel(t, footageServer(t, make(chan string, 1)), withFootageExec(fake), WithConversation("u-1"))
	m = sized(m)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.mode != modeWarn {
		t.Fatalf("missing ffprobe must open the warning overlay, mode=%v", m.mode)
	}
	if cmd != nil {
		t.Error("the gate check is synchronous — no command expected")
	}
	view := m.viewContent()
	if !containsAll(view, "ffprobe", "ffmpeg") {
		t.Fatalf("warning view must explain ffprobe/ffmpeg:\n%s", view)
	}

	// Any key dismisses — not just Esc.
	next, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(Model)
	if m.mode != modeChat {
		t.Fatalf("any key must dismiss the warning, mode=%v", m.mode)
	}
	if m.warn.title != "" || len(m.warn.lines) != 0 {
		t.Errorf("warn state must reset on dismiss: %+v", m.warn)
	}
}

func TestFootageGateFfprobePresentOpensGamePickerTitledFootage(t *testing.T) {
	fake := &fakeFootageExec{}
	m, _ := newTestModel(t, footageServer(t, make(chan string, 1)), withFootageExec(fake), WithConversation("u-1"))
	m = sized(m)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.mode != modeEntityPicker || !m.entity.footage || m.entity.noun != "games" {
		t.Fatalf("ffprobe present must open the footage game picker, mode=%v entity=%+v", m.mode, m.entity)
	}
	m = runCmd(m, cmd)
	if len(m.entity.rows) != 1 {
		t.Fatalf("expected 1 game row, got %d", len(m.entity.rows))
	}
	view := m.entityPickerView()
	if !containsAll(view, "footage") {
		t.Fatalf("game picker must be retitled \"footage\":\n%s", view)
	}
	if !containsAll(view, "update footage — pick the game") {
		t.Fatalf("game picker must say why it's open (owner order 2026-07-13):\n%s", view)
	}
}

func TestFootageCtrlFNoOpWhenUnauthenticated(t *testing.T) {
	fake := &fakeFootageExec{}
	m, _ := newTestModel(t, footageServer(t, make(chan string, 1)), withFootageExec(fake), WithLoginRequired())
	m = sized(m)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(Model)
	if cmd != nil {
		t.Error("an unauthenticated ctrl+f must produce no command")
	}
	if m.mode != modeChat {
		t.Fatalf("an unauthenticated ctrl+f must stay in chat, mode=%v", m.mode)
	}
}

// ── math ────────────────────────────────────────────────────────────────

func TestFootageFileHoursRoundsUpToTheNextHalfHour(t *testing.T) {
	cases := []struct {
		seconds float64
		want    float64
	}{
		{0, 0},
		{-5, 0},
		{1, 0.5},    // any positive duration rounds up to at least 0.5h
		{1800, 0.5}, // exactly 0.5h stays 0.5h
		{1801, 1.0}, // just past 0.5h rounds up to the NEXT half hour
		{3600, 1.0}, // exactly 1h stays 1h
		{3601, 1.5},
		{5400, 1.5}, // exactly 1.5h stays 1.5h
		{7199, 2.0}, // just under 2h rounds up to 2h
	}
	for _, c := range cases {
		if got := footageFileHours(c.seconds); got != c.want {
			t.Errorf("footageFileHours(%v) = %v, want %v", c.seconds, got, c.want)
		}
	}
}

func TestFootageTotalHoursCeilsTheSumToAWholeHour(t *testing.T) {
	cases := []struct {
		sum  float64
		want int
	}{
		{0, 0},
		{0.5, 1},
		{1.0, 1},
		{1.5, 2},
		{2.0, 2},
		{4.0, 4},
	}
	for _, c := range cases {
		if got := footageTotalHours(c.sum); got != c.want {
			t.Errorf("footageTotalHours(%v) = %v, want %v", c.sum, got, c.want)
		}
	}
}

// ── state machine: game → folder → probing → sent ──────────────────────

func TestFootageFlowGameFolderProbingSent(t *testing.T) {
	_, videosDir, _ := folderPickerFixture(t)
	clip1 := filepath.Join(videosDir, "clip1.mp4") // 3600s = 1.0h
	clip2 := filepath.Join(videosDir, "clip2.MKV") // 1800s = 0.5h → total ceil(1.5) = 2

	sent := make(chan string, 1)
	fake := &fakeFootageExec{durations: map[string]float64{clip1: 3600, clip2: 1800}}
	m, _ := newTestModel(t, footageServer(t, sent), withFootageExec(fake), WithConversation("u-1"))
	m = sized(m)

	// 1) ctrl+f → the footage game picker.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(Model)
	m = runCmd(m, cmd)
	if len(m.entity.rows) != 1 {
		t.Fatalf("expected 1 game row, got %d", len(m.entity.rows))
	}

	// 2) picking the game advances to the folder step WITHOUT sending
	// "show game" — the flow remembers the game and opens no chat send.
	next, cmd = m.Update(keyEnter)
	m = next.(Model)
	if cmd != nil {
		t.Error("picking the game must not itself produce a send command")
	}
	if m.mode != modeFootage || m.footage.step != footageStepFolder {
		t.Fatalf("picking the game must open the folder step, mode=%v step=%v", m.mode, m.footage.step)
	}
	if m.footage.gameID != 42 || m.footage.gameTitle != "Elden Ring" {
		t.Fatalf("game not remembered: id=%d title=%q", m.footage.gameID, m.footage.gameTitle)
	}
	select {
	case s := <-sent:
		t.Fatalf("picking the game must not send anything yet, got %q", s)
	default:
	}

	// The folder step must keep the picked game visible (owner order
	// 2026-07-13: "the user never loses sight of what they're probing
	// for") — a persistent breadcrumb naming the title AND the id.
	if view := m.footage.folder.View(); !containsAll(view, "footage → ", "Elden Ring", "#42") {
		t.Fatalf("folder step must show the persistent game breadcrumb:\n%s", view)
	}

	// 3) navigate into Videos/ (started here, the persisted folder — no
	// WithFootageFolder configured, so "" falls back to $HOME; point the
	// picker there directly since this test cares about the probe chain,
	// not the persisted start — TestFootagePersistedFolderRestoresAndSaves
	// covers that). Carry the same breadcrumb the real onFootageGamePicked
	// path wires in, so this substitution stays representative.
	fp := NewFolderPicker(videosDir).WithTruecolor(m.truecolor).
		WithBreadcrumb(footageBreadcrumb(m.footage.gameTitle, m.footage.gameID))
	fp, _ = fp.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.footage.folder = fp

	// Videos/ lists: Sub/ (dir), clip1.mp4, clip2.MKV (dotfile/non-video
	// filtered) — select both files, confirm on the last.
	m = drive(m, keyDown)  // Sub/ -> clip1.mp4
	m = drive(m, keySpace) // select clip1.mp4
	m = drive(m, keyDown)  // clip1.mp4 -> clip2.MKV
	m = drive(m, keySpace) // select clip2.MKV
	next, cmd = m.Update(keyEnter)
	m = next.(Model)
	if m.mode != modeFootage || m.footage.step != footageStepProbing {
		t.Fatalf("confirming the folder must start probing, mode=%v step=%v", m.mode, m.footage.step)
	}
	if len(m.footage.files) != 2 {
		t.Fatalf("expected 2 selected files, got %d: %v", len(m.footage.files), m.footage.files)
	}
	if cmd == nil {
		t.Fatal("confirming the folder must fire the probe (+ persist) commands")
	}

	// 4) drive the probe chain to completion — one message per file,
	// progress readable at each step, the game breadcrumb still pinned.
	view := m.viewContent()
	if !containsAll(view, "0/2") {
		t.Fatalf("probing view must show initial progress 0/2:\n%s", view)
	}
	if !containsAll(view, "footage → ", "Elden Ring", "#42") {
		t.Fatalf("probing view must keep the game breadcrumb visible:\n%s", view)
	}
	var finalCmd tea.Cmd
	for i := 0; i < len(m.footage.files); i++ {
		msg, ok := m.footageProbeCmd(i)().(FootageProbedMsg)
		if !ok {
			t.Fatalf("footageProbeCmd(%d) did not answer a FootageProbedMsg", i)
		}
		next, c := m.Update(msg)
		m = next.(Model)
		finalCmd = c
	}

	// 5) the last probe finalizes: mode closes, the game/folder state
	// resets, and the send command is the ONLY thing left to run.
	if m.mode != modeChat {
		t.Fatalf("the flow must close back to chat once every file is probed, mode=%v", m.mode)
	}
	if m.footage.gameID != 0 || len(m.footage.files) != 0 {
		t.Fatalf("footage state must reset after sending, got %+v", m.footage)
	}
	if finalCmd == nil {
		t.Fatal("the last probe must produce the send command")
	}
	_ = finalCmd()
	select {
	case got := <-sent:
		if got != "update game footage 42 2" {
			t.Fatalf("sent %q, want %q", got, "update game footage 42 2")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the footage update send")
	}
}

func TestFootageFlowSkipsUnreadableFilesAndTalliesThem(t *testing.T) {
	_, videosDir, _ := folderPickerFixture(t)
	clip1 := filepath.Join(videosDir, "clip1.mp4")
	clip2 := filepath.Join(videosDir, "clip2.MKV")

	sent := make(chan string, 1)
	fake := &fakeFootageExec{
		durations:  map[string]float64{clip1: 3600}, // 1.0h
		unreadable: map[string]bool{clip2: true},
	}
	m, _ := newTestModel(t, footageServer(t, sent), withFootageExec(fake), WithConversation("u-1"))
	m = sized(m)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(Model)
	m = runCmd(m, cmd)
	m = drive(m, keyEnter) // pick the one game

	fp := NewFolderPicker(videosDir).WithTruecolor(m.truecolor)
	fp, _ = fp.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.footage.folder = fp
	m = drive(m, keyDown)  // Sub/ -> clip1.mp4
	m = drive(m, keySpace) // select clip1.mp4
	m = drive(m, keyDown)  // clip1.mp4 -> clip2.MKV
	m = drive(m, keySpace) // select clip2.MKV (unreadable)
	next, _ = m.Update(keyEnter)
	m = next.(Model)

	var finalCmd tea.Cmd
	for i := 0; i < len(m.footage.files); i++ {
		msg := m.footageProbeCmd(i)().(FootageProbedMsg)
		next, c := m.Update(msg)
		m = next.(Model)
		finalCmd = c
	}
	if finalCmd == nil {
		t.Fatal("expected the send command")
	}
	_ = finalCmd()
	select {
	case got := <-sent:
		// Only clip1's 1.0h counts — clip2 skipped, contributes 0.
		if got != "update game footage 42 1" {
			t.Fatalf("sent %q, want %q", got, "update game footage 42 1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the footage update send")
	}
	if len(m.notices) == 0 || !containsAll(m.notices[len(m.notices)-1], "1", "skipped") {
		t.Fatalf("a skipped file must surface as a notice: %#v", m.notices)
	}
}

// ── esc cancels cleanly ─────────────────────────────────────────────────

func TestFootageEscCancelsFolderStep(t *testing.T) {
	_, videosDir, _ := folderPickerFixture(t)
	fake := &fakeFootageExec{}
	m, _ := newTestModel(t, footageServer(t, make(chan string, 1)), withFootageExec(fake), WithConversation("u-1"))
	m = sized(m)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(Model)
	m = runCmd(m, cmd)
	m = drive(m, keyEnter) // pick the game → folder step

	fp := NewFolderPicker(videosDir).WithTruecolor(m.truecolor)
	fp, _ = fp.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.footage.folder = fp

	next, _ = m.Update(keyEsc)
	m = next.(Model)
	if m.mode != modeChat {
		t.Fatalf("esc must cancel back to chat, mode=%v", m.mode)
	}
	if m.footage.gameID != 0 || m.footage.gameTitle != "" {
		t.Fatalf("esc must reset the remembered game, got %+v", m.footage)
	}
}

func TestFootageEscCancelsProbingStepAndDropsStaleProbes(t *testing.T) {
	_, videosDir, _ := folderPickerFixture(t)
	clip1 := filepath.Join(videosDir, "clip1.mp4")
	clip2 := filepath.Join(videosDir, "clip2.MKV")
	fake := &fakeFootageExec{durations: map[string]float64{clip1: 3600, clip2: 1800}}
	m, _ := newTestModel(t, footageServer(t, make(chan string, 1)), withFootageExec(fake), WithConversation("u-1"))
	m = sized(m)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(Model)
	m = runCmd(m, cmd)
	m = drive(m, keyEnter)

	fp := NewFolderPicker(videosDir).WithTruecolor(m.truecolor)
	fp, _ = fp.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.footage.folder = fp
	m = drive(m, keyDown, keySpace, keyDown, keySpace) // select clip1 + clip2
	next, _ = m.Update(keyEnter)                       // confirm → probing starts, index 0 in flight
	m = next.(Model)
	if m.footage.step != footageStepProbing {
		t.Fatalf("expected the probing step, got %v", m.footage.step)
	}

	// Cancel before the in-flight probe answers.
	next, _ = m.Update(keyEsc)
	m = next.(Model)
	if m.mode != modeChat {
		t.Fatalf("esc must cancel probing back to chat, mode=%v", m.mode)
	}

	// The stale index-0 answer must land as a no-op, not resurrect the flow.
	staleMsg := FootageProbedMsg{Index: 0, Duration: 3600}
	next, c := m.Update(staleMsg)
	m = next.(Model)
	if m.mode != modeChat || c != nil {
		t.Fatalf("a stale probe answer after cancel must be dropped, mode=%v cmd=%v", m.mode, c)
	}
}

// ── persisted folder ─────────────────────────────────────────────────────

func TestFootagePersistedFolderRestoresAndSaves(t *testing.T) {
	_, videosDir, _ := folderPickerFixture(t)
	saveCalled := make(chan string, 1)
	save := func(folder string) error {
		saveCalled <- folder
		return nil
	}
	fake := &fakeFootageExec{}
	m, _ := newTestModel(t, footageServer(t, make(chan string, 1)), withFootageExec(fake),
		WithFootageFolder(videosDir, save), WithConversation("u-1"))
	m = sized(m)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = next.(Model)
	m = runCmd(m, cmd)
	next, _ = m.Update(keyEnter) // pick the one game
	m = next.(Model)

	if got := m.footage.folder.CurrentPath(); got != videosDir {
		t.Fatalf("folder step must START at the persisted last-used folder %q, got %q", videosDir, got)
	}

	// Confirm on an UNSELECTED file row (folderpicker.go contract #5: enter
	// on a file confirms regardless of selection) — a legal zero-file
	// confirm that still must persist the folder it confirmed IN.
	m = drive(m, keyDown) // Sub/ -> clip1.mp4
	next, cmd = m.Update(keyEnter)
	m = next.(Model)
	m = runCmd(m, cmd)

	select {
	case f := <-saveCalled:
		if f != videosDir {
			t.Fatalf("saved folder = %q, want %q", f, videosDir)
		}
	default:
		t.Fatal("confirming the folder must persist it via the save callback")
	}
	if m.mode != modeChat {
		t.Fatalf("a zero-file confirm must finalize immediately, mode=%v", m.mode)
	}
}

// containsAll reports whether s contains every substring in want.
func containsAll(s string, want ...string) bool {
	for _, w := range want {
		if !strings.Contains(s, w) {
			return false
		}
	}
	return true
}
