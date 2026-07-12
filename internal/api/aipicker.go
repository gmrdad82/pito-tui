// The /config ai model picker's wire pair (tui-needs ask #3, answered
// 2026-07-12): the state READ is GET /settings/ai (JSON via the house
// Accept header) — the exact assembly the web overlay renders, served by
// pito's Ai::PickerState so the two faces cannot drift — and every WRITE
// is PATCH /settings/ai, the same endpoint the web's Stimulus controller
// persists through. The key never round-trips: requests carry it once,
// responses only ever say key_present.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// AiModel is one selectable model of a provider's catalog. Pinned marks
// the static fallback entries (no live list without a key).
type AiModel struct {
	ID     string `json:"id"`
	Pinned bool   `json:"pinned"`
}

// AiProvider is one section of the picker: registry order, label as
// authored in pito's ai_providers.yml. Keyless providers (except
// opencode, which lists unauthenticated) carry Models == [] — the UI
// renders the key-gate copy line instead.
type AiProvider struct {
	Provider   string    `json:"provider"`
	Label      string    `json:"label"`
	KeyPresent bool      `json:"key_present"`
	Reasoning  string    `json:"reasoning"`
	Models     []AiModel `json:"models"`
}

// AiPickerState is the full picker state — the contract's seven keys,
// nothing else, nothing omitted. Favorites/recents/conversation entries
// are "provider/model" strings, newest first where ordered.
type AiPickerState struct {
	Providers          []AiProvider `json:"providers"`
	ActiveProvider     string       `json:"active_provider"`
	ActiveModel        string       `json:"active_model"`
	Effort             string       `json:"effort"`
	Favorites          []string     `json:"favorites"`
	Recents            []string     `json:"recents"`
	ConversationModels []string     `json:"conversation_models"`
}

// FetchAiPicker GETs the picker state. conversation (the active
// conversation's uuid, "" for none) fills ConversationModels — an
// unknown uuid is shrugged off server-side (200, empty list).
func (c *Client) FetchAiPicker(ctx context.Context, conversation string) (*AiPickerState, error) {
	path := "/settings/ai"
	if conversation != "" {
		q := url.Values{}
		q.Set("conversation", conversation)
		path += "?" + q.Encode()
	}
	resp, body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if err := c.checkAuth(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ai picker: unexpected status %d", resp.StatusCode)
	}
	var state AiPickerState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("ai picker: decoding: %w", err)
	}
	return &state, nil
}

// AiSettingsPatch is one PATCH /settings/ai body — any subset; zero
// values are omitted from the wire. Effort "off" clears the active
// model's effort (the server's own cycle vocabulary), so it is a real
// value here, never elided.
type AiSettingsPatch struct {
	Provider string `json:"provider,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	ClearKey bool   `json:"clear_key,omitempty"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	Favorite string `json:"favorite,omitempty"`
}

// AiSettingsResult is the write's echo: the provider addressed, the
// (possibly just-changed) active model, that provider's key presence,
// and the updated effort/favorites/recents.
type AiSettingsResult struct {
	Provider   string   `json:"provider"`
	Model      string   `json:"model"`
	KeyPresent bool     `json:"key_present"`
	Effort     string   `json:"effort"`
	Favorites  []string `json:"favorites"`
	Recents    []string `json:"recents"`
}

// PatchAiSettings applies one write. A 422 carries the server's own
// {error:} word (unknown_model, unknown_provider, …) — surfaced verbatim
// so the UI's status line never invents a diagnosis.
func (c *Client) PatchAiSettings(ctx context.Context, patch AiSettingsPatch) (*AiSettingsResult, error) {
	resp, body, err := c.do(ctx, http.MethodPatch, "/settings/ai", patch)
	if err != nil {
		return nil, err
	}
	if err := c.checkAuth(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil && e.Error != "" {
			return nil, fmt.Errorf("ai settings: %s", e.Error)
		}
		return nil, fmt.Errorf("ai settings: unprocessable")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ai settings: unexpected status %d", resp.StatusCode)
	}
	var result AiSettingsResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("ai settings: decoding: %w", err)
	}
	return &result, nil
}
