// Command toolsgen reads pito's grammar config (config/pito/tools.yml in the
// pito Rails repo) and writes a deterministic, machine-readable JSON snapshot
// to internal/grammar/grammar.json in this repo.
//
// The snapshot is consumed only by tests and docs in pito-tui — see
// internal/grammar's package doc for the "never import from internal/ui or
// internal/api" rule. This tool itself is not part of the runtime binary.
//
// Usage (run from the pito-tui repo root):
//
//	PITO_REPO=/path/to/pito go run ./tools/toolsgen
//
// PITO_REPO defaults to $HOME/Dev/pito. The grammar is read from that
// repo's COMMITTED tree (git show PITO_REF:config/pito/tools.yml,
// PITO_REF defaulting to HEAD) so a dirty pito working tree never bakes
// unreleased grammar into the snapshot. Set PITO_REF=WORKTREE to preview
// uncommitted grammar deliberately.
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

// ---- Output schema — mirrors the typed structs in internal/grammar/grammar.go. ----

type snapshot struct {
	Source         source                          `json:"source"`
	Tools          []tool                          `json:"tools"`
	Segments       map[string]map[string][]segment `json:"segments"`
	Capabilities   capabilities                    `json:"capabilities"`
	Vocabularies   map[string]vocabulary           `json:"vocabularies"`
	UniversalReply []universalReplyAction          `json:"universal_reply"`
}

type source struct {
	Path string `json:"path"`
	Note string `json:"note"`
}

type tool struct {
	Name           string        `json:"name"`
	Aliases        []string      `json:"aliases"`
	Auth           string        `json:"auth,omitempty"`
	HasChat        bool          `json:"has_chat"`
	HasSlash       bool          `json:"has_slash"`
	Internal       bool          `json:"internal,omitempty"`
	UniversalReply bool          `json:"universal_reply"`
	ReplyTargets   []replyTarget `json:"reply_targets"`
}

type replyTarget struct {
	Target  string   `json:"target"`
	Mode    string   `json:"mode"`
	Aliases []string `json:"aliases"`
}

type segment struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	ReplyTarget string   `json:"reply_target,omitempty"`
	Default     bool     `json:"default"`
	Kind        string   `json:"kind,omitempty"`
	Fill        string   `json:"fill,omitempty"`
	EmitIf      string   `json:"emit_if,omitempty"`
}

type capabilities struct {
	Columns map[string][]column `json:"columns"`
	Filters map[string][]filter `json:"filters"`
}

type column struct {
	Name         string   `json:"name"`
	Aliases      []string `json:"aliases"`
	Sortable     bool     `json:"sortable"`
	RequiresWith bool     `json:"requires_with"`
	Default      bool     `json:"default"`
	Internal     bool     `json:"internal"`
	Desc         string   `json:"desc,omitempty"`
}

type filter struct {
	Name       string   `json:"name"`
	Tokens     []string `json:"tokens"`
	Vocabulary string   `json:"vocabulary,omitempty"`
	Scope      string   `json:"scope,omitempty"`
	Desc       string   `json:"desc,omitempty"`
}

type vocabulary struct {
	Members  []string          `json:"members"`
	Synonyms map[string]string `json:"synonyms,omitempty"`
	Fillers  []string          `json:"fillers,omitempty"`
	Resolver string            `json:"resolver,omitempty"`
}

type universalReplyAction struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Kinds   []string `json:"kinds,omitempty"`
}

// ---- main ----

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "toolsgen:", err)
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

	srcRel := filepath.Join("config", "pito", "tools.yml")

	ref := os.Getenv("PITO_REF")
	if ref == "" {
		ref = "HEAD"
	}
	var raw []byte
	var err error
	if ref == "WORKTREE" {
		srcPath := filepath.Join(repo, srcRel)
		raw, err = os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", srcPath, err)
		}
	} else {
		cmd := exec.Command("git", "-C", repo, "show", ref+":"+filepath.ToSlash(srcRel))
		raw, err = cmd.Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return fmt.Errorf("git show %s:%s in %s: %s", ref, srcRel, repo, strings.TrimSpace(string(ee.Stderr)))
			}
			return fmt.Errorf("git show %s:%s in %s: %w", ref, srcRel, repo, err)
		}
	}

	// tools.yml has two exact-duplicate consecutive lines (`introducer: without`
	// appears twice in a row in two spots, under show/analyze's `without` slot).
	// yaml.v3 rejects duplicate mapping keys outright, so drop the redundant
	// repeats before parsing. Any other duplicate consecutive line would be
	// dropped the same way, but a scan of the source found only these two.
	raw = dropDuplicateConsecutiveLines(raw)

	srcDesc := fmt.Sprintf("%s (ref %s)", filepath.Join(repo, srcRel), ref)
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", srcDesc, err)
	}
	if len(doc.Content) == 0 {
		return fmt.Errorf("parsing %s: empty document", srcDesc)
	}
	root := doc.Content[0]

	snap := snapshot{
		Source: source{
			Path: filepath.ToSlash(srcRel),
			Note: "generated by tools/toolsgen — do not edit; re-run: go generate ./...",
		},
		Segments:       map[string]map[string][]segment{},
		Capabilities:   capabilities{Columns: map[string][]column{}, Filters: map[string][]filter{}},
		Vocabularies:   map[string]vocabulary{},
		UniversalReply: buildUniversalReply(mapGet(root, "universal_reply")),
	}
	snap.Vocabularies = buildVocabularies(mapGet(root, "vocabularies"))

	for _, kv := range mapPairs(mapGet(root, "tools")) {
		name := kv[0].Value
		vNode := kv[1]

		snap.Tools = append(snap.Tools, buildTool(name, vNode))

		if segNode := mapGet(vNode, "segments"); segNode != nil {
			snap.Segments[name] = buildSegments(segNode)
		}
		// capabilities.{columns,filters} live only on the `list` tool.
		if capNode := mapGet(vNode, "capabilities"); capNode != nil {
			snap.Capabilities = buildCapabilities(capNode)
		}
	}

	out, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling snapshot: %w", err)
	}
	out = append(out, '\n')

	// Output paths anchor on the module root, not the cwd — go:generate runs
	// with cwd = internal/grammar/, direct invocations run from the repo root.
	repoRoot, err := moduleRoot()
	if err != nil {
		return err
	}

	outPath := filepath.Join(repoRoot, "internal", "grammar", "grammar.json")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(outPath), err)
	}
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outPath, err)
	}

	fmt.Fprintf(os.Stderr, "toolsgen: wrote %s (%d tools, %d vocabularies)\n", outPath, len(snap.Tools), len(snap.Vocabularies))

	return writeInventory(repoRoot, snap)
}

// moduleRoot walks up from the cwd to the directory holding go.mod.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolving cwd: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above cwd — run from within the pito-tui repo")
		}
		dir = parent
	}
}

// ---- tools-inventory.md — the human-readable tool table (owner's item 4) ----

// toolStatus is one entry of the hand-kept overlay docs/claude/tool-status.yml.
type toolStatus struct {
	Status string `yaml:"status"`
	Note   string `yaml:"note"`
}

// writeInventory merges the grammar snapshot with the hand-kept status
// overlay into docs/claude/tools-inventory.md. The overlay (and the doc)
// live in the gitignored coordination dir; when the overlay is absent
// (fresh clone, CI) the doc is skipped — grammar.json alone must reproduce.
func writeInventory(root string, snap snapshot) error {
	overlayPath := filepath.Join(root, "docs", "claude", "tool-status.yml")
	rawOverlay, err := os.ReadFile(overlayPath)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "toolsgen: %s absent — skipping tools-inventory.md\n", overlayPath)
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", overlayPath, err)
	}
	overlay := map[string]toolStatus{}
	if err := yaml.Unmarshal(rawOverlay, &overlay); err != nil {
		return fmt.Errorf("parsing %s: %w", overlayPath, err)
	}

	esc := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ")
	}
	cell := func(s string) string {
		if s == "" {
			return "—"
		}
		return esc(s)
	}

	var b strings.Builder
	b.WriteString("# PITO tool inventory\n\n")
	b.WriteString("<!-- generated by tools/toolsgen — do not edit by hand.\n")
	b.WriteString("     sources: pito config/pito/tools.yml + docs/claude/tool-status.yml (hand-kept overlay).\n")
	b.WriteString("     regenerate: mise exec -- go generate ./... -->\n\n")
	fmt.Fprintf(&b, "%d tools. Status legend: ok · needs-work · empty-data · excluded · server-missing · unvisited.\n\n", len(snap.Tools))
	b.WriteString("| Tool | Aliases | Auth | Surfaces | Reply tools | Status | Note |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")

	matched := map[string]bool{}
	for _, v := range snap.Tools {
		var surfaces []string
		if v.HasChat {
			surfaces = append(surfaces, "chat")
		}
		if v.HasSlash {
			surfaces = append(surfaces, "slash")
		}
		if v.Internal {
			surfaces = append(surfaces, "internal")
		}
		var replies []string
		for _, rt := range v.ReplyTargets {
			replies = append(replies, rt.Target)
		}
		st, ok := overlay[v.Name]
		if !ok {
			// Overlay keys may use an alias (e.g. at-a-glance for glance).
			for _, a := range v.Aliases {
				if s, hit := overlay[a]; hit {
					st, ok = s, true
					matched[a] = true
					break
				}
			}
		} else {
			matched[v.Name] = true
		}
		if !ok {
			st = toolStatus{Status: "unvisited"}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			esc(v.Name), cell(strings.Join(v.Aliases, ", ")), cell(v.Auth),
			cell(strings.Join(surfaces, ", ")), cell(strings.Join(replies, ", ")),
			cell(st.Status), cell(st.Note))
	}

	if len(snap.UniversalReply) > 0 {
		b.WriteString("\n## Universal reply actions\n\n")
		b.WriteString("Offered on every followupable message (not tools; `Share::UniversalActions`):\n\n")
		b.WriteString("| Action | Aliases | Kinds | Status | Note |\n")
		b.WriteString("|---|---|---|---|---|\n")
		for _, a := range snap.UniversalReply {
			st, ok := overlay[a.Name]
			if ok {
				matched[a.Name] = true
			} else {
				st = toolStatus{Status: "unvisited"}
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
				esc(a.Name), cell(strings.Join(a.Aliases, ", ")),
				cell(strings.Join(a.Kinds, ", ")), cell(st.Status), cell(st.Note))
		}
	}

	var orphans []string
	for key := range overlay {
		if !matched[key] {
			orphans = append(orphans, key)
		}
	}
	if len(orphans) > 0 {
		sort.Strings(orphans)
		b.WriteString("\n## Overlay entries with no matching tool\n\n")
		b.WriteString("TUI-side concepts or stale keys — reconcile by hand:\n\n")
		for _, key := range orphans {
			st := overlay[key]
			fmt.Fprintf(&b, "- **%s** — %s", esc(key), cell(st.Status))
			if st.Note != "" {
				fmt.Fprintf(&b, " (%s)", esc(st.Note))
			}
			b.WriteString("\n")
		}
	}

	invPath := filepath.Join(root, "docs", "claude", "tools-inventory.md")
	if err := os.WriteFile(invPath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", invPath, err)
	}
	fmt.Fprintf(os.Stderr, "toolsgen: wrote %s (%d overlay entries, %d orphans)\n", invPath, len(overlay), len(orphans))
	return nil
}

func dropDuplicateConsecutiveLines(raw []byte) []byte {
	lines := strings.Split(string(raw), "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if i > 0 && line == lines[i-1] && strings.TrimSpace(line) != "" {
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}

// ---- yaml.Node helpers (preserve source declaration order for map keys) ----

func resolveAlias(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// mapPairs returns the key/value node pairs of a mapping node in source
// declaration order. Returns nil for a nil node or a non-mapping node.
func mapPairs(n *yaml.Node) [][2]*yaml.Node {
	n = resolveAlias(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	pairs := make([][2]*yaml.Node, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		pairs = append(pairs, [2]*yaml.Node{n.Content[i], n.Content[i+1]})
	}
	return pairs
}

func mapGet(n *yaml.Node, key string) *yaml.Node {
	for _, kv := range mapPairs(n) {
		if kv[0].Value == key {
			return kv[1]
		}
	}
	return nil
}

func decodeStrings(n *yaml.Node) []string {
	out := []string{}
	if n == nil {
		return out
	}
	if err := n.Decode(&out); err != nil {
		panic(fmt.Errorf("decoding string list at line %d: %w", n.Line, err))
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func decodeString(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	var s string
	if err := n.Decode(&s); err != nil {
		return n.Value
	}
	return s
}

func decodeBool(n *yaml.Node, def bool) bool {
	if n == nil {
		return def
	}
	var b bool
	if err := n.Decode(&b); err != nil {
		return def
	}
	return b
}

// ---- tools.yml → snapshot builders ----

func buildUniversalReply(n *yaml.Node) []universalReplyAction {
	out := []universalReplyAction{}
	for _, kv := range mapPairs(n) {
		val := kv[1]
		out = append(out, universalReplyAction{
			Name:    kv[0].Value,
			Aliases: decodeStrings(mapGet(val, "aliases")),
			Kinds:   decodeStrings(mapGet(val, "kinds")),
		})
	}
	return out
}

func buildVocabularies(n *yaml.Node) map[string]vocabulary {
	out := map[string]vocabulary{}
	for _, kv := range mapPairs(n) {
		name := kv[0].Value
		val := kv[1]
		v := vocabulary{
			Members:  decodeStrings(mapGet(val, "members")),
			Fillers:  decodeStrings(mapGet(val, "fillers")),
			Resolver: decodeString(mapGet(val, "resolver")),
		}
		if synNode := mapGet(val, "synonyms"); synNode != nil {
			syn := map[string]string{}
			if err := synNode.Decode(&syn); err != nil {
				panic(fmt.Errorf("vocabulary %q synonyms: %w", name, err))
			}
			v.Synonyms = syn
		}
		out[name] = v
	}
	return out
}

func buildTool(name string, n *yaml.Node) tool {
	v := tool{
		Name:           name,
		Aliases:        decodeStrings(mapGet(n, "aliases")),
		HasChat:        mapGet(n, "chat") != nil,
		HasSlash:       mapGet(n, "slash") != nil,
		Internal:       decodeBool(mapGet(n, "internal"), false),
		UniversalReply: decodeBool(mapGet(n, "universal_reply"), true),
		ReplyTargets:   []replyTarget{},
	}
	// 2.0.0 declares surfaces explicitly via availability:; the
	// block-presence detection above remains the fallback for entries
	// that omit it (reply-only tools).
	if av := mapGet(n, "availability"); av != nil {
		v.HasChat = decodeBool(mapGet(av, "chat"), false)
		v.HasSlash = decodeBool(mapGet(av, "slash"), false)
	}

	// auth precedence: top-level `auth:` (chat tools) > slash.auth (slash-only
	// tools, e.g. login/config/jobs) > chat.auth (unused today, kept as a
	// fallback in case a future tool nests it there).
	switch {
	case mapGet(n, "auth") != nil:
		v.Auth = decodeString(mapGet(n, "auth"))
	case mapGet(n, "slash") != nil && mapGet(mapGet(n, "slash"), "auth") != nil:
		v.Auth = decodeString(mapGet(mapGet(n, "slash"), "auth"))
	case mapGet(n, "chat") != nil && mapGet(mapGet(n, "chat"), "auth") != nil:
		v.Auth = decodeString(mapGet(mapGet(n, "chat"), "auth"))
	}

	if targetsNode := mapGet(mapGet(n, "reply"), "targets"); targetsNode != nil {
		for _, kv := range mapPairs(targetsNode) {
			tval := kv[1]
			v.ReplyTargets = append(v.ReplyTargets, replyTarget{
				Target:  kv[0].Value,
				Mode:    decodeString(mapGet(tval, "mode")),
				Aliases: decodeStrings(mapGet(tval, "aliases")),
			})
		}
	}

	return v
}

func buildSegments(n *yaml.Node) map[string][]segment {
	out := map[string][]segment{}
	for _, kv := range mapPairs(n) {
		noun := kv[0].Value
		segs := []segment{}
		for _, skv := range mapPairs(kv[1]) {
			sval := skv[1]
			segs = append(segs, segment{
				Name:        skv[0].Value,
				Aliases:     decodeStrings(mapGet(sval, "aliases")),
				ReplyTarget: decodeString(mapGet(sval, "reply_target")),
				Default:     decodeBool(mapGet(sval, "default"), false),
				Kind:        decodeString(mapGet(sval, "kind")),
				Fill:        decodeString(mapGet(sval, "fill")),
				EmitIf:      decodeString(mapGet(sval, "emit_if")),
			})
		}
		out[noun] = segs
	}
	return out
}

func buildCapabilities(n *yaml.Node) capabilities {
	caps := capabilities{Columns: map[string][]column{}, Filters: map[string][]filter{}}

	for _, kv := range mapPairs(mapGet(n, "columns")) {
		noun := kv[0].Value
		cols := []column{}
		for _, ckv := range mapPairs(kv[1]) {
			cval := ckv[1]
			cols = append(cols, column{
				Name:         ckv[0].Value,
				Aliases:      decodeStrings(mapGet(cval, "aliases")),
				Sortable:     decodeBool(mapGet(cval, "sortable"), false),
				RequiresWith: decodeBool(mapGet(cval, "requires_with"), false),
				Default:      decodeBool(mapGet(cval, "default"), false),
				Internal:     decodeBool(mapGet(cval, "internal"), false),
				Desc:         decodeString(mapGet(cval, "desc")),
			})
		}
		caps.Columns[noun] = cols
	}

	for _, kv := range mapPairs(mapGet(n, "filters")) {
		noun := kv[0].Value
		filters := []filter{}
		for _, fkv := range mapPairs(kv[1]) {
			fval := fkv[1]
			filters = append(filters, filter{
				Name:       fkv[0].Value,
				Tokens:     decodeStrings(mapGet(fval, "tokens")),
				Vocabulary: decodeString(mapGet(fval, "vocabulary")),
				Scope:      decodeString(mapGet(fval, "scope")),
				Desc:       decodeString(mapGet(fval, "desc")),
			})
		}
		caps.Filters[noun] = filters
	}

	return caps
}
