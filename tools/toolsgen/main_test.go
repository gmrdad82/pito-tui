package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// grammarDoc parses a tools.yml-shaped snippet into its root mapping node.
func grammarDoc(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Content) == 0 {
		t.Fatal("empty document")
	}
	return doc.Content[0]
}

func TestBuildersReadTheGrammarShape(t *testing.T) {
	root := grammarDoc(t, `
universal_reply:
  share:  { mode: append, kinds: [system, enhanced] }
  revoke: { mode: append, aliases: [unshare], kinds: [system] }
vocabularies:
  genres:
    members: [rpg, arcade]
    synonyms: { "role-playing": rpg }
tools:
  list:
    aliases: [ls]
    auth: session
    chat:
      dispatch: Chat::Handlers::List
    capabilities:
      columns:
        vids:
          publish_at: { aliases: [publish], sortable: true, requires_with: true }
          scheduled:  { internal: true }
      filters:
        games:
          genre: { vocabulary: genres, scope: any }
  show:
    auth: session
    chat: {}
    reply:
      targets:
        game_detail: { mode: append, aliases: [detail] }
    segments:
      game:
        similar: { aliases: [alike], reply_target: game_similar, default: true }
  login:
    slash: { auth: anonymous }
`)

	ur := buildUniversalReply(mapGet(root, "universal_reply"))
	if len(ur) != 2 || ur[1].Name != "revoke" || ur[1].Aliases[0] != "unshare" {
		t.Errorf("universal reply misparsed: %+v", ur)
	}

	vocab := buildVocabularies(mapGet(root, "vocabularies"))
	if g := vocab["genres"]; len(g.Members) != 2 || g.Synonyms["role-playing"] != "rpg" {
		t.Errorf("vocabulary misparsed: %+v", vocab)
	}

	var tools []tool
	var caps capabilities
	segs := map[string]map[string][]segment{}
	for _, kv := range mapPairs(mapGet(root, "tools")) {
		tools = append(tools, buildTool(kv[0].Value, kv[1]))
		if n := mapGet(kv[1], "segments"); n != nil {
			segs[kv[0].Value] = buildSegments(n)
		}
		if n := mapGet(kv[1], "capabilities"); n != nil {
			caps = buildCapabilities(n)
		}
	}

	if tools[0].Name != "list" || tools[0].Aliases[0] != "ls" || !tools[0].HasChat || tools[0].HasSlash {
		t.Errorf("list tool misparsed: %+v", tools[0])
	}
	if tools[1].ReplyTargets[0].Target != "game_detail" || tools[1].ReplyTargets[0].Aliases[0] != "detail" {
		t.Errorf("reply targets misparsed: %+v", tools[1].ReplyTargets)
	}
	// login's auth nests under slash: — the fallback chain must find it.
	if tools[2].Auth != "anonymous" || !tools[2].HasSlash {
		t.Errorf("slash auth fallback misparsed: %+v", tools[2])
	}

	cols := caps.Columns["vids"]
	if len(cols) != 2 || !cols[0].Sortable || !cols[0].RequiresWith || cols[0].Aliases[0] != "publish" {
		t.Errorf("columns misparsed: %+v", cols)
	}
	if !cols[1].Internal {
		t.Errorf("internal column flag lost: %+v", cols[1])
	}
	if f := caps.Filters["games"]; len(f) != 1 || f[0].Vocabulary != "genres" || f[0].Scope != "any" {
		t.Errorf("filters misparsed: %+v", caps.Filters)
	}

	sg := segs["show"]["game"]
	if len(sg) != 1 || sg[0].Name != "similar" || !sg[0].Default || sg[0].ReplyTarget != "game_similar" {
		t.Errorf("segments misparsed: %+v", segs)
	}
}

func TestDropDuplicateConsecutiveLines(t *testing.T) {
	in := "a:\n  introducer: without\n  introducer: without\n  other: x\n\n\nb: 1\n"
	out := string(dropDuplicateConsecutiveLines([]byte(in)))
	if strings.Count(out, "introducer: without") != 1 {
		t.Errorf("duplicate line survived:\n%s", out)
	}
	if !strings.Contains(out, "other: x") || !strings.Contains(out, "b: 1") {
		t.Errorf("non-duplicate lines lost:\n%s", out)
	}
	// Blank-line runs are NOT deduped (only non-blank duplicates are).
	if !strings.Contains(out, "\n\n\n") {
		t.Errorf("blank run was collapsed:\n%s", out)
	}
}

func TestModuleRootFindsGoMod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "internal", "grammar")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	got, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	// TempDir may ride a symlink (macOS/tmpfs); compare resolved paths.
	want, _ := filepath.EvalSymlinks(root)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != want {
		t.Errorf("moduleRoot() = %q, want %q", got, want)
	}
}

func TestWriteInventoryMergesTheOverlay(t *testing.T) {
	snap := snapshot{
		Tools: []tool{
			{Name: "list", Aliases: []string{"ls"}, Auth: "session", HasChat: true,
				ReplyTargets: []replyTarget{{Target: "game_list", Mode: "mutate"}}},
			{Name: "greet", HasChat: true},
		},
		UniversalReply: []universalReplyAction{
			{Name: "share", Kinds: []string{"system"}},
		},
	}

	root := t.TempDir()
	dir := filepath.Join(root, "docs", "claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := "list: {status: ok, note: \"pipes | escape\"}\nshare: {status: ok}\nghost: {status: stale}\n"
	if err := os.WriteFile(filepath.Join(dir, "tool-status.yml"), []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeInventory(root, snap); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "tools-inventory.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(out)

	if !strings.Contains(doc, `| list | ls | session | chat | game_list | ok | pipes \| escape |`) {
		t.Errorf("list row wrong:\n%s", doc)
	}
	if !strings.Contains(doc, "| greet | — | — | chat | — | unvisited | — |") {
		t.Errorf("unvisited row wrong:\n%s", doc)
	}
	if !strings.Contains(doc, "## Universal reply actions") || !strings.Contains(doc, "| share |") {
		t.Errorf("universal reply section missing:\n%s", doc)
	}
	if !strings.Contains(doc, "## Overlay entries with no matching tool") || !strings.Contains(doc, "**ghost**") {
		t.Errorf("orphan section missing:\n%s", doc)
	}
}

func TestWriteInventorySkipsWithoutOverlay(t *testing.T) {
	root := t.TempDir()
	if err := writeInventory(root, snapshot{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "claude", "tools-inventory.md")); !os.IsNotExist(err) {
		t.Error("inventory must not be written when the overlay is absent")
	}
}
