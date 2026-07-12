//go:build live

package app

// Live contract spec for the /config ai picker pair (tui-needs ask #3):
// GET /settings/ai must serve the seven-key state the TUI modal renders,
// and PATCH /settings/ai must round-trip a favorite toggle (applied on
// the first write, reverted by the second — net zero on dev's settings).
// Excluded from CI; run:
//
//	go test -tags live -run TestAiPickerContract -v ./internal/app/

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/config"
)

func TestAiPickerContract(t *testing.T) {
	instance := os.Getenv("PITO_INSTANCE")
	if instance == "" {
		instance = "https://dev.pitomd.com"
	}
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	client, err := api.New(instance, filepath.Join(dir, "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	state, err := client.FetchAiPicker(ctx, "")
	if err == api.ErrUnauthorized {
		// The dev instance's TOTP login (dev-only credential).
		if err := client.Login(ctx, "123456"); err != nil {
			t.Fatalf("dev login: %v", err)
		}
		state, err = client.FetchAiPicker(ctx, "")
	}
	if err != nil {
		t.Fatalf("state read: %v", err)
	}

	if len(state.Providers) == 0 {
		t.Fatal("providers must list the registry")
	}
	var opencode *api.AiProvider
	for i := range state.Providers {
		if state.Providers[i].Provider == "opencode" {
			opencode = &state.Providers[i]
		}
		if state.Providers[i].Label == "" {
			t.Errorf("provider %q carries no label", state.Providers[i].Provider)
		}
	}
	if opencode == nil {
		t.Fatal("the opencode provider must be in the registry")
	}
	if len(opencode.Models) == 0 {
		t.Fatal("opencode lists models unauthenticated — empty means the live list broke")
	}
	if state.ActiveProvider == "" {
		t.Fatal("active_provider must always resolve (default opencode)")
	}

	// Favorite round-trip: toggle on, verify, toggle off, verify — the
	// second write restores dev's settings exactly.
	entry := opencode.Provider + "/" + opencode.Models[0].ID
	had := contains(state.Favorites, entry)
	first, err := client.PatchAiSettings(ctx, api.AiSettingsPatch{Favorite: entry})
	if err != nil {
		t.Fatalf("favorite toggle: %v", err)
	}
	if contains(first.Favorites, entry) == had {
		t.Fatalf("first toggle must flip %q (had=%v): %v", entry, had, first.Favorites)
	}
	second, err := client.PatchAiSettings(ctx, api.AiSettingsPatch{Favorite: entry})
	if err != nil {
		t.Fatalf("favorite revert: %v", err)
	}
	if contains(second.Favorites, entry) != had {
		t.Fatalf("second toggle must restore %q (had=%v): %v", entry, had, second.Favorites)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
