// Package grammar loads a machine-readable snapshot of pito's chat/slash
// grammar — parsed from the pito Rails app's config/pito/tools.yml by
// tools/toolsgen — so pito-tui's tests and docs can assert against the real
// tool/alias/segment/capability names instead of hardcoding copies that rot
// when pito renames or adds things.
//
// THIS PACKAGE IS FOR TESTS AND TOOLS ONLY. It must never be imported from
// internal/ui or internal/api: the runtime pito-tui binary does not carry
// grammar knowledge, and grammar_test.go enforces that guarantee.
//
// Regenerate the snapshot with `go generate ./...` (or directly via
// `go run ./tools/toolsgen`) after pito's tools.yml changes.
//
// The snapshot is normally PINNED to the pito release this TUI version
// pairs with — PITO_REF names that tag and toolsgen reads the committed
// blob (git show <ref>:config/pito/tools.yml), so WIP in the pito
// checkout can't leak in.
//
// Currently pinned to pito v4.0.0 — re-pin deliberately (and regenerate)
// when pairing with a newer pito release; don't let it drift silently.
package grammar

//go:generate env PITO_REF=v4.0.0 go run github.com/gmrdad82/pito-tui/tools/toolsgen

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed grammar.json
var grammarJSON []byte

// Grammar is the root of the generated snapshot — see tools/toolsgen/main.go
// for the code that produces it from tools.yml, and grammar.json for the
// current snapshot.
type Grammar struct {
	Source         Source                          `json:"source"`
	Tools          []Tool                          `json:"tools"`
	Segments       map[string]map[string][]Segment `json:"segments"`
	Capabilities   Capabilities                    `json:"capabilities"`
	Vocabularies   map[string]Vocabulary           `json:"vocabularies"`
	UniversalReply []UniversalReplyAction          `json:"universal_reply"`
}

// Source identifies the pito file this snapshot was generated from.
type Source struct {
	Path string `json:"path"`
	Note string `json:"note"`
}

// Tool is one entry from tools.yml's top-level `tools:` map.
type Tool struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	// Auth is the tool's auth requirement (e.g. "session",
	// "authenticated_only", "unauthenticated_only"). Resolution order:
	// top-level `auth:` (chat tools), then `slash.auth` (slash-only tools),
	// then `chat.auth`. Empty when none of those are set.
	Auth string `json:"auth,omitempty"`
	// HasChat/HasSlash come from the tool's `availability:` block when
	// declared (2.0.0+), falling back to `chat:`/`slash:` block presence.
	HasChat  bool `json:"has_chat"`
	HasSlash bool `json:"has_slash"`
	// Internal marks reply tools not shown in user-facing palettes (e.g.
	// `consume`).
	Internal bool `json:"internal,omitempty"`
	// UniversalReply is false only when the tool opts out of the global
	// share/revoke reply actions via `universal_reply: false` (e.g. `sync`).
	UniversalReply bool          `json:"universal_reply"`
	ReplyTargets   []ReplyTarget `json:"reply_targets"`
}

// ReplyTarget is one entry from a tool's `reply.targets` map.
type ReplyTarget struct {
	Target  string   `json:"target"`
	Mode    string   `json:"mode"`
	Aliases []string `json:"aliases"`
}

// Segment is one entry from a show/analyze `segments.<noun>` map.
type Segment struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	ReplyTarget string   `json:"reply_target,omitempty"`
	Default     bool     `json:"default"`
	Kind        string   `json:"kind,omitempty"`
	Fill        string   `json:"fill,omitempty"`
	EmitIf      string   `json:"emit_if,omitempty"`
}

// Capabilities holds the `list` tool's capabilities block: the columns and
// filters available per noun (games/vids/channels).
type Capabilities struct {
	Columns map[string][]Column `json:"columns"`
	Filters map[string][]Filter `json:"filters"`
}

// Column is one entry from capabilities.columns.<noun>.
type Column struct {
	Name         string   `json:"name"`
	Aliases      []string `json:"aliases"`
	Sortable     bool     `json:"sortable"`
	RequiresWith bool     `json:"requires_with"`
	Default      bool     `json:"default"`
	Internal     bool     `json:"internal"`
	Desc         string   `json:"desc,omitempty"`
}

// Filter is one entry from capabilities.filters.<noun>.
type Filter struct {
	Name       string   `json:"name"`
	Tokens     []string `json:"tokens"`
	Vocabulary string   `json:"vocabulary,omitempty"`
	Scope      string   `json:"scope,omitempty"`
	Desc       string   `json:"desc,omitempty"`
}

// Vocabulary is one entry from the top-level `vocabularies:` map.
type Vocabulary struct {
	Members  []string          `json:"members"`
	Synonyms map[string]string `json:"synonyms,omitempty"`
	Fillers  []string          `json:"fillers,omitempty"`
	// Resolver is set instead of Members/Synonyms for dynamic vocabularies
	// (e.g. "channels", "game_titles") resolved at runtime in Ruby.
	Resolver string `json:"resolver,omitempty"`
}

// UniversalReplyAction is one entry from the top-level `universal_reply:`
// map (share/revoke) — the reply actions offered on followupable events
// unless a tool opts out (see Tool.UniversalReply).
type UniversalReplyAction struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Kinds   []string `json:"kinds,omitempty"`
}

// Load parses the embedded grammar.json snapshot.
func Load() (Grammar, error) {
	var g Grammar
	if err := json.Unmarshal(grammarJSON, &g); err != nil {
		return Grammar{}, fmt.Errorf("grammar: parsing embedded grammar.json: %w", err)
	}
	return g, nil
}
