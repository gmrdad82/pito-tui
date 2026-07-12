package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, filepath.Join(t.TempDir(), "cookies.json"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestFetchChatDecodesAndSendsHeaders(t *testing.T) {
	fixture := readFixture(t, "chat_page.json")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /chat/abc.json", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	})
	c := newClient(t, mux)

	page, err := c.FetchChat(t.Context(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 6 {
		t.Errorf("events = %d", len(page.Events))
	}
}

func TestFetchChatUnauthorized(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	_, err := c.FetchChat(t.Context(), "abc")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestFetchChatRedirectMeansUnauthorized(t *testing.T) {
	// Rails auth stacks 302 to a login page instead of 401ing; the client
	// must not follow it and must classify it as an auth failure.
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}))
	_, err := c.FetchChat(t.Context(), "abc")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestFetchResume(t *testing.T) {
	fixture := readFixture(t, "resume.json")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	})
	c := newClient(t, mux)

	list, err := c.FetchResume(t.Context(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Recent) != 2 || len(list.Older) != 1 {
		t.Errorf("recent/older = %d/%d", len(list.Recent), len(list.Older))
	}
}

// TestFetchResumeFirstPageOmitsQueryEntirely pins the deliberate deviation
// from FetchNotifications' idiom: an unpaginated call (after=="", limit<=0)
// must send NO query string at all, so a pre-pagination server sees the
// exact request it has always answered.
func TestFetchResumeFirstPageOmitsQueryEntirely(t *testing.T) {
	fixture := readFixture(t, "resume.json")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("RawQuery = %q, want empty — old servers must see a byte-identical request", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	})
	c := newClient(t, mux)

	if _, err := c.FetchResume(t.Context(), "", 0); err != nil {
		t.Fatal(err)
	}
}

func TestFetchResumeSecondPageCarriesCursorAndLimit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("after"); got != "cursor-2" {
			t.Errorf("after = %q, want %q", got, "cursor-2")
		}
		if got := r.URL.Query().Get("limit"); got != "10" {
			t.Errorf("limit = %q, want %q", got, "10")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"older":[],"next_cursor":null}`))
	})
	c := newClient(t, mux)

	if _, err := c.FetchResume(t.Context(), "cursor-2", 10); err != nil {
		t.Fatal(err)
	}
}

func TestFetchResumeNextCursorDecodes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"recent":[],"older":[],"next_cursor":"abc"}`))
	})
	c := newClient(t, mux)

	list, err := c.FetchResume(t.Context(), "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if list.NextCursor != "abc" {
		t.Errorf("next_cursor = %q, want %q", list.NextCursor, "abc")
	}
}

func TestFetchResumeNullNextCursorMeansExhausted(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"recent":[],"older":[],"next_cursor":null}`))
	})
	c := newClient(t, mux)

	list, err := c.FetchResume(t.Context(), "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if list.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty (null means exhausted)", list.NextCursor)
	}
}

// TestFetchResumeOldShapeResponseYieldsEmptyNextCursor covers a genuine
// pre-2.0.0 server: no `next_cursor` key at all. It must decode as a
// complete, exhausted page rather than erroring.
func TestFetchResumeOldShapeResponseYieldsEmptyNextCursor(t *testing.T) {
	fixture := readFixture(t, "resume.json")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	})
	c := newClient(t, mux)

	list, err := c.FetchResume(t.Context(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if list.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty (absent means exhausted)", list.NextCursor)
	}
}

// TestFetchResumeFlatRowsAbsorbedAsOlder pins the past-page-1 flattening
// tolerance: a `rows` array folds onto the end of Older alongside whatever
// `older` rows also arrived, so callers never special-case the flat shape.
func TestFetchResumeFlatRowsAbsorbedAsOlder(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"older": [{"uuid": "already-older", "title": "already older"}],
			"rows": [
				{"uuid": "flat-1", "title": "flat one"},
				{"uuid": "flat-2", "title": "flat two"}
			],
			"next_cursor": "next-page"
		}`))
	})
	c := newClient(t, mux)

	list, err := c.FetchResume(t.Context(), "cursor-2", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Recent) != 0 {
		t.Errorf("recent = %d, want 0 — later pages never carry recent rows", len(list.Recent))
	}
	if len(list.Older) != 3 {
		t.Fatalf("older = %d, want 3 (1 grouped + 2 flattened)", len(list.Older))
	}
	wantUUIDs := []string{"already-older", "flat-1", "flat-2"}
	for i, want := range wantUUIDs {
		if list.Older[i].UUID != want {
			t.Errorf("older[%d].UUID = %q, want %q", i, list.Older[i].UUID, want)
		}
	}
}

func TestFetchNotificationsFirstPageOmitsBefore(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notifications.json", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("after"); got != "" {
			t.Errorf("before = %q, want omitted on the first page", got)
		}
		if _, present := r.URL.Query()["after"]; present {
			t.Error("before must not appear in the query at all on the first page")
		}
		if got := r.URL.Query().Get("limit"); got != "50" {
			t.Errorf("limit = %q, want default 50", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rows":[{"id":123,"message":"hi","read":false,"created_at":"2026-07-10T12:00:00Z"}],"next_cursor":"abc"}`))
	})
	c := newClient(t, mux)

	page, err := c.FetchNotifications(t.Context(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(page.Rows))
	}
	row := page.Rows[0]
	if row.ID != 123 || row.Message != "hi" || row.Read != false {
		t.Errorf("row = %+v", row)
	}
	wantCreatedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	if !row.CreatedAt.Equal(wantCreatedAt) {
		t.Errorf("created_at = %v, want %v", row.CreatedAt, wantCreatedAt)
	}
	if page.NextCursor != "abc" {
		t.Errorf("next_cursor = %q, want %q", page.NextCursor, "abc")
	}
}

func TestFetchNotificationsSecondPageCarriesCursorAndLimit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notifications.json", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("after"); got != "cursor-2" {
			t.Errorf("before = %q, want %q", got, "cursor-2")
		}
		if got := r.URL.Query().Get("limit"); got != "10" {
			t.Errorf("limit = %q, want %q", got, "10")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rows":[],"next_cursor":null}`))
	})
	c := newClient(t, mux)

	page, err := c.FetchNotifications(t.Context(), "cursor-2", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 0 {
		t.Errorf("rows = %d, want 0", len(page.Rows))
	}
	if page.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty (null means exhausted)", page.NextCursor)
	}
}

func TestFetchNotificationsAbsentNextCursorMeansExhausted(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notifications.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rows":[]}`))
	})
	c := newClient(t, mux)

	page, err := c.FetchNotifications(t.Context(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty (absent means exhausted)", page.NextCursor)
	}
}

func TestFetchNotificationsNotAcceptableIsUnavailable(t *testing.T) {
	// Rails answers 406 (not 404) when the route exists for html only —
	// live-verified against dev before the endpoint rolled out.
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotAcceptable)
	}))
	_, err := c.FetchNotifications(t.Context(), "", 0)
	if !errors.Is(err, ErrNotificationsUnavailable) {
		t.Fatalf("406 must map to ErrNotificationsUnavailable, got %v", err)
	}
}

func TestFetchNotificationsUnavailable(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := c.FetchNotifications(t.Context(), "", 0)
	if !errors.Is(err, ErrNotificationsUnavailable) {
		t.Fatalf("err = %v, want ErrNotificationsUnavailable", err)
	}
}

func TestFetchNotificationsUnauthorized(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	_, err := c.FetchNotifications(t.Context(), "", 0)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestSendMessageAck(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["input"] != "show game 5" || body["uuid"] != "abc" {
			t.Errorf("body = %v", body)
		}
		if body["viewport_width"] != float64(120) {
			t.Errorf("viewport_width = %v, want 120", body["viewport_width"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		// Live-verified: the server echoes the uuid on every ack.
		_, _ = w.Write([]byte(`{"uuid":"abc","turn_id":9}`))
	})
	c := newClient(t, mux)

	res, err := c.SendMessage(t.Context(), "abc", "show game 5", 120)
	if err != nil {
		t.Fatal(err)
	}
	if res.TurnID != 9 || res.CreatedUUID != "" || res.Notice != nil {
		t.Errorf("result = %+v — an echoed uuid on a normal ack must NOT read as created", res)
	}
}

func TestSendMessageBlankUUIDCreatesConversation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, present := body["uuid"]; present {
			t.Error("blank uuid must be omitted from the body entirely")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"uuid":"fresh-uuid","turn_id":42}`))
	})
	c := newClient(t, mux)

	res, err := c.SendMessage(t.Context(), "", "hello", 80)
	if err != nil {
		t.Fatal(err)
	}
	if res.CreatedUUID != "fresh-uuid" || res.TurnID != 42 {
		t.Errorf("result = %+v, want created uuid AND its turn id", res)
	}
}

func TestSendMessageWebOnlyNotice(t *testing.T) {
	for name, status := range map[string]int{"200": http.StatusOK, "422": http.StatusUnprocessableEntity} {
		t.Run(name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"web_only","message":"That command wears a mouse cursor. Wrong outfit for here."}`))
			})
			c := newClient(t, mux)

			res, err := c.SendMessage(t.Context(), "abc", "/themes", 80)
			if err != nil {
				t.Fatal(err)
			}
			if res.Notice == nil || res.Notice.Text() != "That command wears a mouse cursor. Wrong outfit for here." {
				t.Errorf("result = %+v, want the server's own notice prose", res)
			}
		})
	}
}

func TestSendMessageUnauthorized(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	_, err := c.SendMessage(t.Context(), "abc", "hi", 80)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestLoginMintsCookieIntoJar(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			OTP string `json:"otp"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.OTP != "123456" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "pito_session", Value: "minted", Path: "/"})
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /chat/abc.json", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("pito_session"); err != nil || c.Value != "minted" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"conversation":{"uuid":"abc","title":""},"events":[]}`))
	})
	c := newClient(t, mux)

	if err := c.Login(t.Context(), "123456"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FetchChat(t.Context(), "abc"); err != nil {
		t.Fatalf("fetch after login = %v — cookie did not stick", err)
	}
}

func TestLoginErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"invalid 401", http.StatusUnauthorized, `{"error":"invalid"}`, ErrInvalidTOTP},
		{"throttled 429", http.StatusTooManyRequests, ``, ErrThrottled},
		{"throttled by body", http.StatusUnprocessableEntity, `{"error":"throttled"}`, ErrThrottled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			if err := c.Login(t.Context(), "000000"); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestJarAndBaseURLAccessors(t *testing.T) {
	c := newClient(t, http.NewServeMux())
	if c.Jar() == nil {
		t.Error("Jar() must expose the shared jar for the cable dialer")
	}
	if c.BaseURL() == nil || c.BaseURL().Host == "" {
		t.Error("BaseURL() must return the instance URL")
	}
}
