package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func gamesClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := New(srv.URL, filepath.Join(t.TempDir(), "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestSearchIGDBHappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/games/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/games/search" {
			t.Errorf("path = %q, want /games/search", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if body.Query != "hollow" {
			t.Errorf("query = %q, want %q", body.Query, "hollow")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"hits":[{"id":1,"name":"Hollow Knight","type_note":""},{"id":2,"name":"Hollow Knight (remake)","type_note":"(remake)"}],"error":null,"library_ids":[2]}`)
	})
	client := gamesClient(t, mux)

	result, err := client.SearchIGDB(t.Context(), "hollow", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(result.Hits))
	}
	if result.Hits[0] != (IgdbHit{ID: 1, Name: "Hollow Knight", TypeNote: ""}) {
		t.Errorf("hits[0] = %+v", result.Hits[0])
	}
	if result.Hits[1] != (IgdbHit{ID: 2, Name: "Hollow Knight (remake)", TypeNote: "(remake)"}) {
		t.Errorf("hits[1] = %+v", result.Hits[1])
	}
	if !result.LibraryIDs[2] {
		t.Error("LibraryIDs[2] = false, want true")
	}
	if result.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", result.ErrorMessage)
	}
}

func TestSearchIGDBErrorEnvelopeIsNotAGoError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/games/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"hits":[],"error":{"kind":"upstream_unavailable","message":"IGDB is down"},"library_ids":[]}`)
	})
	client := gamesClient(t, mux)

	result, err := client.SearchIGDB(t.Context(), "hollow", 0)
	if err != nil {
		t.Fatalf("degraded answer must not be a Go error, got %v", err)
	}
	if len(result.Hits) != 0 {
		t.Errorf("hits = %d, want 0", len(result.Hits))
	}
	if result.ErrorMessage != "IGDB is down" {
		t.Errorf("ErrorMessage = %q, want %q", result.ErrorMessage, "IGDB is down")
	}
}

func TestSearchIGDBUnauthorized(t *testing.T) {
	client := gamesClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	_, err := client.SearchIGDB(t.Context(), "hollow", 0)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestSearchIGDBNon200IsStatusError(t *testing.T) {
	client := gamesClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	_, err := client.SearchIGDB(t.Context(), "hollow", 0)
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("err = %v, want *StatusError", err)
	}
	if statusErr.Code != http.StatusInternalServerError {
		t.Errorf("Code = %d, want %d", statusErr.Code, http.StatusInternalServerError)
	}
}

func TestImportGameHappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/games/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/games/import" {
			t.Errorf("path = %q, want /games/import", r.URL.Path)
		}
		var body struct {
			IgdbID int    `json:"igdb_id"`
			Title  string `json:"title"`
			UUID   string `json:"uuid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if body.IgdbID != 7 || body.Title != "Celeste" || body.UUID != "abc-123" {
			t.Errorf("body = %+v, want {IgdbID:7 Title:Celeste UUID:abc-123}", body)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	client := gamesClient(t, mux)

	if err := client.ImportGame(t.Context(), 7, "Celeste", "abc-123"); err != nil {
		t.Fatalf("ImportGame = %v, want nil", err)
	}
}

func TestImportGameUnprocessableIsStatusError(t *testing.T) {
	client := gamesClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))

	err := client.ImportGame(t.Context(), 7, "Celeste", "abc-123")
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("err = %v, want *StatusError", err)
	}
	if statusErr.Code != http.StatusUnprocessableEntity {
		t.Errorf("Code = %d, want %d", statusErr.Code, http.StatusUnprocessableEntity)
	}
}

func TestImportGameUnauthorized(t *testing.T) {
	client := gamesClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	err := client.ImportGame(t.Context(), 7, "Celeste", "abc-123")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

// TestSearchIGDBPositiveLimitSetsQueryParam pins the other half of
// SearchIGDB's limit contract (viewport-driven paging, owner 2026-07-15):
// a positive limit rides the query string verbatim.
func TestSearchIGDBPositiveLimitSetsQueryParam(t *testing.T) {
	var gotLimit string
	mux := http.NewServeMux()
	mux.HandleFunc("/games/search", func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"hits":[]}`)
	})
	client := gamesClient(t, mux)

	if _, err := client.SearchIGDB(t.Context(), "hollow", 24); err != nil {
		t.Fatal(err)
	}
	if gotLimit != "24" {
		t.Errorf("limit = %q, want %q", gotLimit, "24")
	}
}

// TestSearchIGDBNonPositiveLimitOmitsQueryParam pins the <=0 fallback
// documented on SearchIGDB: the param is omitted entirely rather than
// sent as 0 or negative, so the server falls back to the tool's
// configured default.
func TestSearchIGDBNonPositiveLimitOmitsQueryParam(t *testing.T) {
	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprintf("limit=%d", limit), func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/games/search", func(w http.ResponseWriter, r *http.Request) {
				if _, present := r.URL.Query()["limit"]; present {
					t.Errorf("limit query param present (%q), want omitted for limit=%d", r.URL.RawQuery, limit)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"hits":[]}`)
			})
			client := gamesClient(t, mux)

			if _, err := client.SearchIGDB(t.Context(), "hollow", limit); err != nil {
				t.Fatal(err)
			}
		})
	}
}
