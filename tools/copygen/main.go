// Command copygen reads pito's user-facing copy pools — the thinking
// word dictionaries, the clipboard toast quips, the start-screen tips,
// the ctrl+k palette strings and the chatbox cycler-hint strings — from
// pito's locale files and writes ONE deterministic JSON snapshot to
// internal/ui/render/pito_copy.json in this repo (owner COPY LAW
// 2026-07-12: every user-facing word is authored in pito; the TUI only
// mirrors, never invents).
//
// Unlike toolsgen's grammar snapshot (tests/docs-only), this one IS
// embedded in the runtime binary: the pools are presentation copy — the
// words the web's ThinkingComponent cycles while a turn runs — not
// grammar. The server stays the single author (this file is generated,
// never hand-edited); the client merely replays the payload's own
// dictionary/order/started_at against the same word lists, exactly like
// the web's Stimulus controller replays them from data attributes.
//
// Usage (run from the pito-tui repo root):
//
//	PITO_REPO=/path/to/pito go run ./tools/copygen
//
// PITO_REPO defaults to $HOME/Dev/pito. The copy is read from that
// repo's COMMITTED tree (git show PITO_REF:config/locales/pito/copy/en.yml,
// PITO_REF defaulting to HEAD) so a dirty pito working tree never bakes
// unreleased copy into the snapshot. Set PITO_REF=WORKTREE to preview
// uncommitted copy deliberately.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// snapshot mirrors what internal/ui/render/thinking.go consumes. Word
// ORDER is load-bearing: the payload's `order`/`word_index` fields index
// into these arrays, so they must stay exactly as authored in pito.
type snapshot struct {
	Source       source                `json:"source"`
	Dictionaries map[string]dictionary `json:"dictionaries"`
	// Clipboard is the TUI-only toast pool (en.pito.copy.tui.clipboard)
	// shown when a mouse selection auto-copies. Optional: an older pito
	// ref without the key yields an empty list and the toast degrades to
	// its glyph alone (COPY LAW: the client never substitutes words).
	Clipboard   []string    `json:"clipboard"`
	StartScreen startScreen `json:"start_screen"`
	Palette     palette     `json:"palette"`
	AiPicker    aiPicker    `json:"ai_picker"`
	// ScrollbackNav: the floating scroll-position pills
	// (en.pito.copy.scrollback_nav) — one fixed string per side
	// (%{count} the only token, owner 2026-07-13: the 50-variant pool
	// retired — pills read identically everywhere, web + tui) and the
	// two jump glyphs.
	ScrollbackNav scrollbackNav `json:"scrollback_nav"`
	Shell         shell         `json:"shell"`
}

type scrollbackNav struct {
	Before      string `json:"before"`
	After       string `json:"after"`
	JumpToStart string `json:"jump_to_start"`
	JumpToEnd   string `json:"jump_to_end"`
}

// startScreen: the boot tip line (en.pito.copy.start_screen.tips +
// en.pito.start_screen.tip_prefix — two different locale files, one
// surface).
type startScreen struct {
	TipPrefix string   `json:"tip_prefix"`
	Tips      []string `json:"tips"`
}

// palette: the ctrl+k command palette strings (en.pito.palette.ctrl_k).
// Item STRUCTURE (sections, inserts) is code in pito's CommandCatalog and
// is ported in internal/ui — only the words live here.
type palette struct {
	Title             string            `json:"title"`
	EscHint           string            `json:"esc_hint"`
	SearchPlaceholder string            `json:"search_placeholder"`
	Sections          map[string]string `json:"sections"`
	Commands          map[string]string `json:"commands"`
}

// aiPicker: the /config ai model picker strings — the modal chrome from
// en.pito.palette.ai_picker plus the keyless-provider gate line from
// en.pito.copy.ai.picker.key_gate (two locale files, one surface).
// Optional like the other sibling pools: an older ref yields zero
// values (COPY LAW: absent words render as absent, never substituted).
type aiPicker struct {
	Title             string            `json:"title"`
	EscHint           string            `json:"esc_hint"`
	SearchPlaceholder string            `json:"search_placeholder"`
	NoModel           string            `json:"no_model"`
	Sections          map[string]string `json:"sections"`
	KeyGate           string            `json:"key_gate"`
}

// shell: the chatbox cycler-hint + mini-status strings (en.pito.shell).
type shell struct {
	ChannelShortcut string `json:"channel_shortcut"`
	PeriodShortcut  string `json:"period_shortcut"`
	NoChannels      string `json:"no_channels"`
	// Anonymous is the mini-status identity shown while unauthenticated
	// (the web's "tarnished").
	Anonymous string `json:"anonymous"`
}

type source struct {
	Path string `json:"path"`
	Note string `json:"note"`
}

type dictionary struct {
	Doing []string `json:"doing"`
	Done  []string `json:"done"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "copygen:", err)
		os.Exit(1)
	}
}

func run() error {
	repo := os.Getenv("PITO_REPO")
	if repo == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolving PITO_REPO default: %w", err)
		}
		repo = filepath.Join(home, "Dev", "pito")
	}

	ref := os.Getenv("PITO_REF")
	if ref == "" {
		ref = "HEAD"
	}

	copyRaw, err := readLocale(repo, ref, "copy")
	if err != nil {
		return err
	}
	snap, err := extract(copyRaw, "config/locales/pito/{copy,palette,start_screen,shell}/en.yml")
	if err != nil {
		return fmt.Errorf("extracting copy pools (ref %s): %w", ref, err)
	}
	// The sibling locale files are OPTIONAL (older pito refs) — each
	// degrades to its zero value and the consuming surface degrades with
	// it (COPY LAW: absent words render as absent, never substituted).
	if raw, err := readLocale(repo, ref, "palette"); err == nil {
		snap.Palette = extractPalette(raw)
		snap.AiPicker = extractAiPicker(raw, copyRaw)
	}
	if raw, err := readLocale(repo, ref, "start_screen"); err == nil {
		snap.StartScreen.TipPrefix = extractTipPrefix(raw)
	}
	if raw, err := readLocale(repo, ref, "shell"); err == nil {
		snap.Shell = extractShell(raw)
	}

	out, err := marshal(snap)
	if err != nil {
		return err
	}

	root, err := moduleRoot()
	if err != nil {
		return err
	}
	dst := filepath.Join(root, "internal", "ui", "render", "pito_copy.json")
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	fmt.Printf("copygen: wrote %s (%d dictionaries, %d tips, %d palette commands)\n",
		dst, len(snap.Dictionaries), len(snap.StartScreen.Tips), len(snap.Palette.Commands))
	return nil
}

// readLocale reads config/locales/pito/<name>/en.yml at the pinned ref
// (or the worktree with PITO_REF=WORKTREE), the same way toolsgen reads
// tools.yml.
func readLocale(repo, ref, name string) ([]byte, error) {
	srcRel := filepath.Join("config", "locales", "pito", name, "en.yml")
	if ref == "WORKTREE" {
		raw, err := os.ReadFile(filepath.Join(repo, srcRel))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filepath.Join(repo, srcRel), err)
		}
		return raw, nil
	}
	cmd := exec.Command("git", "-C", repo, "show", ref+":"+filepath.ToSlash(srcRel))
	raw, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git show %s:%s in %s: %s", ref, srcRel, repo, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git show %s:%s in %s: %w", ref, srcRel, repo, err)
	}
	return raw, nil
}

// yamlRoot unmarshals and returns the document's root mapping node.
func yamlRoot(raw []byte) *yaml.Node {
	var doc yaml.Node
	if yaml.Unmarshal(raw, &doc) != nil || len(doc.Content) == 0 {
		return nil
	}
	return doc.Content[0]
}

// walk descends a chain of mapping keys, nil-safe at every hop.
func walk(node *yaml.Node, keys ...string) *yaml.Node {
	for _, key := range keys {
		node = mapValue(node, key)
	}
	return node
}

// stringMap flattens a mapping of scalar values ({key: "text", …}).
func stringMap(node *yaml.Node) map[string]string {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	out := make(map[string]string, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		out[node.Content[i].Value] = node.Content[i+1].Value
	}
	return out
}

func scalar(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

func extractPalette(raw []byte) palette {
	node := walk(yamlRoot(raw), "en", "pito", "palette", "ctrl_k")
	return palette{
		Title:             scalar(mapValue(node, "title")),
		EscHint:           scalar(mapValue(node, "esc_hint")),
		SearchPlaceholder: scalar(mapValue(node, "search_placeholder")),
		Sections:          stringMap(mapValue(node, "sections")),
		Commands:          stringMap(mapValue(node, "commands")),
	}
}

func extractTipPrefix(raw []byte) string {
	return scalar(walk(yamlRoot(raw), "en", "pito", "start_screen", "tip_prefix"))
}

// extractAiPicker reads the /config ai picker chrome from the palette
// file and its key-gate line from the copy file — the same two sources
// Pito::Ai::PickerComponent + ProviderSectionComponent render from.
func extractAiPicker(paletteRaw, copyRaw []byte) aiPicker {
	node := walk(yamlRoot(paletteRaw), "en", "pito", "palette", "ai_picker")
	return aiPicker{
		Title:             scalar(mapValue(node, "title")),
		EscHint:           scalar(mapValue(node, "esc_hint")),
		SearchPlaceholder: scalar(mapValue(node, "search_placeholder")),
		NoModel:           scalar(mapValue(node, "no_model")),
		Sections:          stringMap(mapValue(node, "sections")),
		KeyGate:           scalar(walk(yamlRoot(copyRaw), "en", "pito", "copy", "ai", "picker", "key_gate")),
	}
}

func extractShell(raw []byte) shell {
	shellNode := walk(yamlRoot(raw), "en", "pito", "shell")
	chatbox := mapValue(shellNode, "chatbox")
	return shell{
		ChannelShortcut: scalar(walk(chatbox, "filter", "channel_shortcut")),
		PeriodShortcut:  scalar(walk(chatbox, "filter", "period_shortcut")),
		NoChannels:      scalar(mapValue(chatbox, "no_channels")),
		Anonymous:       scalar(walk(shellNode, "mini_status", "anonymous")),
	}
}

// extract walks en > pito > copy > thinking and collects every dictionary
// (slash, confirmation, importing, …) that carries doing/done sequences.
func extract(raw []byte, srcPath string) (snapshot, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return snapshot{}, fmt.Errorf("parsing yaml: %w", err)
	}
	if len(doc.Content) == 0 {
		return snapshot{}, fmt.Errorf("empty document")
	}
	root := doc.Content[0]
	node := root
	for _, key := range []string{"en", "pito", "copy"} {
		next := mapValue(node, key)
		if next == nil {
			return snapshot{}, fmt.Errorf("key %q not found", key)
		}
		node = next
	}
	copyNode := node
	node = mapValue(copyNode, "thinking")
	if node == nil {
		return snapshot{}, fmt.Errorf("key %q not found", "thinking")
	}
	clipboard := stringSeq(mapValue(mapValue(copyNode, "tui"), "clipboard"))
	tips := stringSeq(walk(copyNode, "start_screen", "tips"))
	nav := scrollbackNav{
		Before:      scalar(walk(copyNode, "scrollback_nav", "before")),
		After:       scalar(walk(copyNode, "scrollback_nav", "after")),
		JumpToStart: scalar(walk(copyNode, "scrollback_nav", "jump_to_start")),
		JumpToEnd:   scalar(walk(copyNode, "scrollback_nav", "jump_to_end")),
	}

	dicts := map[string]dictionary{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		name := node.Content[i].Value
		body := node.Content[i+1]
		d := dictionary{
			Doing: stringSeq(mapValue(body, "doing")),
			Done:  stringSeq(mapValue(body, "done")),
		}
		if len(d.Doing) == 0 {
			continue // not a word-pool dictionary
		}
		dicts[name] = d
	}
	if len(dicts) == 0 {
		return snapshot{}, fmt.Errorf("no dictionaries under thinking")
	}
	return snapshot{
		Source: source{
			Path: srcPath,
			Note: "generated by tools/copygen — do not edit; re-run: go generate ./...",
		},
		Dictionaries:  dicts,
		Clipboard:     clipboard,
		StartScreen:   startScreen{Tips: tips},
		ScrollbackNav: nav,
	}, nil
}

// marshal renders deterministic JSON: dictionary keys sorted, word arrays
// in source order (order is load-bearing — see snapshot's doc comment).
func marshal(snap snapshot) ([]byte, error) {
	names := make([]string, 0, len(snap.Dictionaries))
	for name := range snap.Dictionaries {
		names = append(names, name)
	}
	sort.Strings(names)
	ordered := make(map[string]dictionary, len(names))
	for _, name := range names {
		ordered[name] = snap.Dictionaries[name]
	}
	snap.Dictionaries = ordered // encoding/json sorts map keys itself
	out, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling: %w", err)
	}
	return append(out, '\n'), nil
}

func mapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func stringSeq(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		out = append(out, item.Value)
	}
	return out
}

// moduleRoot locates the repo root by walking up from the working
// directory to the nearest go.mod — same convention as toolsgen, so the
// generator works from any subdirectory.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found above %s", dir)
		}
		dir = parent
	}
}
