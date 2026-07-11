package grammar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	g, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(g.Verbs) == 0 {
		t.Fatal("Load() returned a grammar with no verbs")
	}
}

func TestLoad_ListVerbHasAliasLs(t *testing.T) {
	g, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	var list *Verb
	for i := range g.Verbs {
		if g.Verbs[i].Name == "list" {
			list = &g.Verbs[i]
			break
		}
	}
	if list == nil {
		t.Fatal(`verb "list" not found in snapshot`)
	}
	if !contains(list.Aliases, "ls") {
		t.Errorf(`verb "list" aliases = %v, want to contain "ls"`, list.Aliases)
	}
}

func TestLoad_SegmentsShowGameContainsSimilar(t *testing.T) {
	g, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	gameSegments, ok := g.Segments["show"]["game"]
	if !ok {
		t.Fatal(`segments.show.game not found in snapshot`)
	}

	var names []string
	found := false
	for _, s := range gameSegments {
		names = append(names, s.Name)
		if s.Name == "similar" {
			found = true
		}
	}
	if !found {
		t.Errorf(`segments.show.game = %v, want to contain "similar"`, names)
	}
}

func TestLoad_CapabilitiesColumnsVidsContainsPublishAt(t *testing.T) {
	g, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	vidColumns, ok := g.Capabilities.Columns["vids"]
	if !ok {
		t.Fatal(`capabilities.columns.vids not found in snapshot`)
	}

	var names []string
	found := false
	for _, c := range vidColumns {
		names = append(names, c.Name)
		if c.Name == "publish_at" {
			found = true
		}
	}
	if !found {
		t.Errorf(`capabilities.columns.vids = %v, want to contain "publish_at"`, names)
	}
}

func TestLoad_VocabulariesPlatformsHasMemberPC(t *testing.T) {
	g, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	platforms, ok := g.Vocabularies["platforms"]
	if !ok {
		t.Fatal(`vocabularies.platforms not found in snapshot`)
	}
	if !contains(platforms.Members, "PC") {
		t.Errorf(`vocabularies.platforms.members = %v, want to contain "PC"`, platforms.Members)
	}
}

// TestRuntimeStaysGrammarFree guards the promise in this package's doc
// comment: internal/ui and internal/api must never import internal/grammar.
// The snapshot is a tool/test-only convenience — the runtime pito-tui binary
// must not gain grammar knowledge from it.
func TestRuntimeStaysGrammarFree(t *testing.T) {
	const forbidden = "internal/grammar"

	for _, dir := range []string{
		filepath.Join("..", "ui"),
		filepath.Join("..", "api"),
	} {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(b), forbidden) {
				t.Errorf("%s references %q — the runtime binary (internal/ui, internal/api) must stay grammar-free; internal/grammar is for tests/tools only", path, forbidden)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", dir, err)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
