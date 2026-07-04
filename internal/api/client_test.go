package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
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

func TestResume(t *testing.T) {
	fixture := readFixture(t, "resume.json")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /resume.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	})
	c := newClient(t, mux)

	list, err := c.Resume(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Recent) != 2 || len(list.Older) != 1 {
		t.Errorf("recent/older = %d/%d", len(list.Recent), len(list.Older))
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
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":true,"turn_id":9}`))
	})
	c := newClient(t, mux)

	res, err := c.SendMessage(t.Context(), "abc", "show game 5", 120)
	if err != nil {
		t.Fatal(err)
	}
	if res.TurnID != 9 || res.CreatedUUID != "" || res.WebOnly != nil {
		t.Errorf("result = %+v", res)
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
		_, _ = w.Write([]byte(`{"uuid":"fresh-uuid"}`))
	})
	c := newClient(t, mux)

	res, err := c.SendMessage(t.Context(), "", "hello", 80)
	if err != nil {
		t.Fatal(err)
	}
	if res.CreatedUUID != "fresh-uuid" {
		t.Errorf("CreatedUUID = %q", res.CreatedUUID)
	}
}

func TestSendMessageWebOnlyNotice(t *testing.T) {
	for name, status := range map[string]int{"200": http.StatusOK, "422": http.StatusUnprocessableEntity} {
		t.Run(name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"web-only","verb":"/themes"}`))
			})
			c := newClient(t, mux)

			res, err := c.SendMessage(t.Context(), "abc", "/themes", 80)
			if err != nil {
				t.Fatal(err)
			}
			if res.WebOnly == nil || res.WebOnly.Verb != "/themes" {
				t.Errorf("result = %+v, want a web-only notice", res)
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
		_, _ = w.Write([]byte(`{"conversation":{"uuid":"abc","name":""},"events":[]}`))
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
