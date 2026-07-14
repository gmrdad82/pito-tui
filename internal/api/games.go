// The HTTP face of the games-import flow — the TUI side of the web's
// games-import sidebar (Games::SearchController / Games::ImportController).
// Search is plain JSON in both directions; import is fire-and-forget (the
// server answers 204 and narrates progress into the conversation over the
// cable as the ordinary announce/done messages the transcript renders).
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// IgdbHit is one /games/search result row. Field names are pinned by the
// web overlay's own consumption (hit.id, hit.name, hit.type_note —
// games_search_controller.js).
type IgdbHit struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	TypeNote string `json:"type_note"`
}

// IgdbSearch is the /games/search envelope: hits plus the igdb ids already
// in the local library (the picker labels those — picking one re-syncs
// rather than duplicating, the Importer's own semantics).
type IgdbSearch struct {
	Hits       []IgdbHit
	LibraryIDs map[int]bool
	// ErrorMessage carries the server's error envelope (IGDB credentials
	// missing, upstream down) — a degraded ANSWER, not a transport failure.
	ErrorMessage string
}

// SearchIGDB POSTs {query} to /games/search.
func (c *Client) SearchIGDB(ctx context.Context, query string) (*IgdbSearch, error) {
	resp, body, err := c.do(ctx, http.MethodPost, "/games/search", map[string]string{"query": query})
	if err != nil {
		return nil, err
	}
	if err := c.checkAuth(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Method: "POST", Path: "/games/search", Code: resp.StatusCode, Status: resp.Status}
	}
	var raw struct {
		Hits  []IgdbHit `json:"hits"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		LibraryIDs []int `json:"library_ids"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("api: decoding games search: %w", err)
	}
	out := &IgdbSearch{Hits: raw.Hits, LibraryIDs: make(map[int]bool, len(raw.LibraryIDs))}
	for _, id := range raw.LibraryIDs {
		out.LibraryIDs[id] = true
	}
	if raw.Error != nil {
		out.ErrorMessage = raw.Error.Message
	}
	return out, nil
}

// ImportGame POSTs the picked hit to /games/import. 204 means the import
// job is queued; everything after arrives in the scrollback.
func (c *Client) ImportGame(ctx context.Context, igdbID int, title, conversationUUID string) error {
	payload := map[string]any{"igdb_id": igdbID, "title": title, "uuid": conversationUUID}
	resp, _, err := c.do(ctx, http.MethodPost, "/games/import", payload)
	if err != nil {
		return err
	}
	if err := c.checkAuth(resp); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return &StatusError{Method: "POST", Path: "/games/import", Code: resp.StatusCode, Status: resp.Status}
	}
	return nil
}
