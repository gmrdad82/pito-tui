package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func aiPickerClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := New(srv.URL, t.TempDir()+"/jar.json")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

const aiStateFixture = `{
	"providers": [
		{"provider": "opencode", "label": "OpenCode Zen", "key_present": true,
		 "reasoning": "effort",
		 "models": [{"id": "claude-fable-5", "pinned": false}, {"id": "deepseek-v4-flash-free", "pinned": true}]},
		{"provider": "anthropic", "label": "Anthropic", "key_present": false,
		 "reasoning": "none", "models": []}
	],
	"active_provider": "opencode",
	"active_model": "deepseek-v4-flash-free",
	"effort": null,
	"favorites": ["opencode/claude-fable-5"],
	"recents": ["opencode/deepseek-v4-flash-free"],
	"conversation_models": ["opencode/deepseek-v4-flash-free"]
}`

func TestFetchAiPickerDecodesTheContract(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/settings/ai", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("state read must GET, got %s", r.Method)
		}
		if got := r.URL.Query().Get("conversation"); got != "uuid-9" {
			t.Errorf("conversation param = %q, want uuid-9", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, aiStateFixture)
	})
	client := aiPickerClient(t, mux)
	state, err := client.FetchAiPicker(context.Background(), "uuid-9")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Providers) != 2 || state.Providers[0].Label != "OpenCode Zen" {
		t.Fatalf("providers decoded wrong: %+v", state.Providers)
	}
	if !state.Providers[0].KeyPresent || state.Providers[1].KeyPresent {
		t.Fatal("key_present flags lost in decode")
	}
	if state.Providers[0].Models[1].Pinned != true {
		t.Fatal("pinned flag lost")
	}
	if state.ActiveModel != "deepseek-v4-flash-free" || state.Effort != "" {
		t.Fatalf("active/effort wrong: %q %q", state.ActiveModel, state.Effort)
	}
	if len(state.Favorites) != 1 || len(state.ConversationModels) != 1 {
		t.Fatal("favorites/conversation_models lost")
	}
}

func TestFetchAiPickerAnonymousIsUnauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/settings/ai", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	client := aiPickerClient(t, mux)
	if _, err := client.FetchAiPicker(context.Background(), ""); err != ErrUnauthorized {
		t.Fatalf("anonymous read must surface ErrUnauthorized, got %v", err)
	}
}

func TestPatchAiSettingsSendsSubsetsAndDecodesEcho(t *testing.T) {
	var got map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/settings/ai", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("writes must PATCH, got %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"provider":"opencode","model":"claude-fable-5","key_present":true,
			"effort":"high","favorites":["opencode/claude-fable-5"],"recents":["opencode/claude-fable-5"]}`)
	})
	client := aiPickerClient(t, mux)
	result, err := client.PatchAiSettings(context.Background(), AiSettingsPatch{Provider: "opencode", Model: "claude-fable-5"})
	if err != nil {
		t.Fatal(err)
	}
	// The wire body carries ONLY the subset: no api_key, no clear_key,
	// no effort/favorite ghosts (a stray clear_key would nuke the key).
	if len(got) != 2 || got["provider"] != "opencode" || got["model"] != "claude-fable-5" {
		t.Fatalf("patch body must be the exact subset, got %v", got)
	}
	if result.Model != "claude-fable-5" || result.Effort != "high" || !result.KeyPresent {
		t.Fatalf("echo decoded wrong: %+v", result)
	}
}

func TestPatchAiSettingsSurfacesTheServersErrorWord(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/settings/ai", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"error":"unknown_model"}`)
	})
	client := aiPickerClient(t, mux)
	_, err := client.PatchAiSettings(context.Background(), AiSettingsPatch{Provider: "opencode", Model: "ghost"})
	if err == nil || err.Error() != "ai settings: unknown_model" {
		t.Fatalf("422 must carry the server's own error word, got %v", err)
	}
}
