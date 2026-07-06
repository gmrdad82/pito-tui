//go:build live

package app

// Live contract spec for the SHOW screen — the detail cards (`show game/
// vid/channel`) and their reply surface. Detail replies are append-mode
// (verbs.yml): `#handle <verb>` adds a new message below the card. Only
// side-effect-free verbs run here (analyze, shinies, and an invalid
// action for the error path) — delete/publish/link stay out of a spec
// that runs against real library state. Run:
//
//	go test -tags live -run TestShowReplyContract -v -timeout 600s ./internal/app/
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/config"
)

type showState struct {
	handle    string
	body      string
	lastID    int64
	numEvents int
}

func fetchShowState(t *testing.T, client *api.Client, uuid string) showState {
	t.Helper()
	page, err := client.FetchChat(t.Context(), uuid)
	if err != nil {
		t.Fatal(err)
	}
	st := showState{numEvents: len(page.Events)}
	for _, ev := range page.Events {
		if ev.ID > st.lastID {
			st.lastID = ev.ID
		}
		var p struct {
			Body        string `json:"body"`
			ReplyHandle string `json:"reply_handle"`
		}
		if json.Unmarshal(ev.Payload, &p) != nil {
			continue
		}
		if strings.Contains(p.Body, "pito-detail-stats") && p.ReplyHandle != "" {
			st.handle, st.body = p.ReplyHandle, p.Body
		}
	}
	return st
}

func waitShow(t *testing.T, client *api.Client, uuid string, check func(showState) bool) (showState, bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last showState
	for time.Now().Before(deadline) {
		last = fetchShowState(t, client, uuid)
		if check(last) {
			return last, true
		}
		time.Sleep(700 * time.Millisecond)
	}
	return last, false
}

func TestShowReplyContract(t *testing.T) {
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

	cases := []struct {
		noun    string
		command string
		// body fragments the detail card must carry (the TUI parses these).
		wants []string
		// a safe append reply to fire; "" skips.
		reply string
	}{
		{"game", "show game #1", []string{"pito-game-detail", "pito-score-bar", "pito-ttb", "grid-cols", "__description"}, "shinies"},
		{"vid", "show vid #28", []string{"pito-video-detail", "grid-cols", "__description"}, "analyze"},
		{"channel", "show channel @gmrdad82", []string{"pito-channel-detail", "grid-cols"}, "analyze"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.noun, func(t *testing.T) {
			time.Sleep(1200 * time.Millisecond)
			res, err := client.SendMessage(ctx, "", tc.command, 1000)
			if err != nil || res.CreatedUUID == "" {
				t.Fatalf("%s: %v %+v", tc.command, err, res)
			}
			uuid := res.CreatedUUID
			st, ok := waitShow(t, client, uuid, func(s showState) bool { return s.handle != "" })
			if !ok {
				t.Fatal("detail card never materialized")
			}
			for _, want := range tc.wants {
				if !strings.Contains(st.body, want) {
					t.Errorf("detail body missing %q", want)
				}
			}

			// Append reply: a NEW event must arrive below the card.
			if tc.reply != "" {
				before := st.numEvents
				if _, err := client.SendMessage(ctx, uuid, fmt.Sprintf("#%s %s", st.handle, tc.reply), 1000); err != nil {
					t.Fatalf("reply %s: %v", tc.reply, err)
				}
				if _, ok := waitShow(t, client, uuid, func(s showState) bool { return s.numEvents > before+1 }); !ok {
					t.Errorf("reply %q: no appended events arrived", tc.reply)
				}
			}

			// Invalid action → error event (the reply router's error path).
			before := fetchShowState(t, client, uuid).numEvents
			if _, err := client.SendMessage(ctx, uuid, fmt.Sprintf("#%s frobnicate", st.handle), 1000); err != nil {
				t.Fatalf("invalid reply: %v", err)
			}
			if _, ok := waitShow(t, client, uuid, func(s showState) bool { return s.numEvents > before }); !ok {
				t.Errorf("invalid action never produced a reply event")
			}
		})
	}
}

// TestGameSegmentsContract covers the game's enhanced segments: similar
// (recommendations reply with `show`), channels (async coverage fill
// reaches ready), and videos — the linked-videos segment, RENAMED in
// pito's verbs update (2026-07-06: game `linked-videos`→`videos`, vid
// `linked-game`→`game`) — a Video::List with the full mutate reply
// surface: with/without columns and visible-column sorts.
func TestGameSegmentsContract(t *testing.T) {
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

	res, err := client.SendMessage(ctx, "", "show game #1 with similar, channels, videos", 1000)
	if err != nil || res.CreatedUUID == "" {
		t.Fatalf("show with segments: %v %+v", err, res)
	}
	uuid := res.CreatedUUID

	type segState struct {
		similarHandle, similarGameID string
		channelsReady                bool
		numEvents                    int
	}
	fetchSegs := func() segState {
		page, err := client.FetchChat(t.Context(), uuid)
		if err != nil {
			t.Fatal(err)
		}
		st := segState{numEvents: len(page.Events)}
		for _, ev := range page.Events {
			var p struct {
				Body         string `json:"body"`
				ReplyHandle  string `json:"reply_handle"`
				ReplyTarget  string `json:"reply_target"`
				Distribution struct {
					Status string `json:"status"`
				} `json:"channel_distribution"`
			}
			if json.Unmarshal(ev.Payload, &p) != nil {
				continue
			}
			switch p.ReplyTarget {
			case "game_similar":
				st.similarHandle = p.ReplyHandle
				if m := regexp.MustCompile(`data-game-id="(\d+)"`).FindStringSubmatch(p.Body); m != nil {
					st.similarGameID = m[1]
				}
			case "game_channels":
				st.channelsReady = p.Distribution.Status == "ready"
			}
		}
		return st
	}
	waitSegs := func(check func(segState) bool) (segState, bool) {
		deadline := time.Now().Add(30 * time.Second)
		var last segState
		for time.Now().Before(deadline) {
			last = fetchSegs()
			if check(last) {
				return last, true
			}
			time.Sleep(900 * time.Millisecond)
		}
		return last, false
	}

	// All three segments land; the channels fill reaches ready.
	st, ok := waitSegs(func(s segState) bool { return s.similarHandle != "" && s.channelsReady })
	if !ok {
		t.Fatalf("segments never completed: %+v", st)
	}
	if st.similarGameID == "" {
		t.Fatal("similar strip carries no game ids")
	}

	// Linked-videos FIRST: mutate replies (with/sort/without) emit no new
	// system turn, so every handle stays live through them. A reply that
	// APPENDS (the similar `show` below) retires ALL prior hashtags
	// (finalizer.rb consume_prior_live_replies) — order matters.
	list, ok := waitListState(t, client, uuid, 0, 12*time.Second, func(s listState) bool { return s.handle != "" })
	if !ok {
		t.Fatal("linked-videos list never materialized")
	}
	send := func(input string) {
		t.Helper()
		if _, err := client.SendMessage(ctx, uuid, input, 120); err != nil {
			t.Fatalf("send %q: %v", input, err)
		}
	}
	send(fmt.Sprintf("#%s with visibility", list.handle))
	if _, ok := waitListState(t, client, uuid, list.eventID, 12*time.Second, func(s listState) bool { return s.hasHeading("Visibility") }); !ok {
		t.Error("with visibility: heading never appeared")
	}
	send(fmt.Sprintf("#%s sort duration desc", list.handle))
	descSt, _ := waitListState(t, client, uuid, list.eventID, 8*time.Second, func(s listState) bool {
		return s.firstCell("Duration") != list.firstCell("Duration")
	})
	send(fmt.Sprintf("#%s sort duration", list.handle))
	if _, ok := waitListState(t, client, uuid, list.eventID, 15*time.Second, func(s listState) bool {
		return s.firstCell("Duration") != descSt.firstCell("Duration")
	}); !ok {
		t.Errorf("sort duration: ascending never led with a different Duration than descending (stuck at %q)", descSt.firstCell("Duration"))
	}
	send(fmt.Sprintf("#%s without duration", list.handle))
	if _, ok := waitListState(t, client, uuid, list.eventID, 12*time.Second, func(s listState) bool { return !s.hasHeading("Duration") }); !ok {
		t.Error("without duration: heading never left")
	}

	// Recommendations reply LAST: `#handle show #<id>` appends the game's
	// card below — and, being a new leading turn, retires every prior
	// hashtag in the conversation.
	before := fetchSegs().numEvents
	if _, err := client.SendMessage(ctx, uuid, fmt.Sprintf("#%s show #%s", st.similarHandle, st.similarGameID), 1000); err != nil {
		t.Fatalf("similar show reply: %v", err)
	}
	if _, ok := waitSegs(func(s segState) bool { return s.numEvents > before }); !ok {
		t.Error("similar `show` reply appended nothing")
	}
	// The sweep: the linked list's handle must now be consumed. The echo
	// arrives before the system event that triggers the sweep — poll.
	linkedConsumed := func() bool {
		page, err := client.FetchChat(t.Context(), uuid)
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range page.Events {
			if ev.ID != list.eventID {
				continue
			}
			var p struct {
				ReplyConsumed bool `json:"reply_consumed"`
			}
			return json.Unmarshal(ev.Payload, &p) == nil && p.ReplyConsumed
		}
		return false
	}
	deadline := time.Now().Add(12 * time.Second)
	swept := false
	for time.Now().Before(deadline) {
		if swept = linkedConsumed(); swept {
			break
		}
		time.Sleep(700 * time.Millisecond)
	}
	if !swept {
		t.Errorf("prior linked handle should be consumed after an appending reply")
	}
}

// TestGlanceContract covers the at-a-glance segment for all three nouns
// (AnalyticsFillJob pending → ready with braille sparkline cells) and
// the vid's linked-game card (game_detail reply surface included).
func TestGlanceContract(t *testing.T) {
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

	cases := []struct {
		noun    string
		command string
		linked  bool
	}{
		{"channel", "show channel @gmrdad82 with at-a-glance", false},
		{"vid", "show vid #28 with at-a-glance, game", true},
		{"game", "show game #1 with at-a-glance", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.noun, func(t *testing.T) {
			time.Sleep(1200 * time.Millisecond)
			res, err := client.SendMessage(ctx, "", tc.command, 1000)
			if err != nil || res.CreatedUUID == "" {
				t.Fatalf("%s: %v %+v", tc.command, err, res)
			}
			uuid := res.CreatedUUID

			type glanceState struct {
				handle, body, linkedHandle string
				ready                      bool
				numEvents                  int
			}
			fetchGlance := func() glanceState {
				page, err := client.FetchChat(t.Context(), uuid)
				if err != nil {
					t.Fatal(err)
				}
				st := glanceState{numEvents: len(page.Events)}
				for _, ev := range page.Events {
					var p struct {
						Body        string `json:"body"`
						ReplyHandle string `json:"reply_handle"`
						ReplyTarget string `json:"reply_target"`
						Analytics   struct {
							Status string `json:"status"`
						} `json:"analytics"`
					}
					if json.Unmarshal(ev.Payload, &p) != nil {
						continue
					}
					if p.ReplyTarget == "analytics_glance" {
						st.handle, st.body = p.ReplyHandle, p.Body
						st.ready = p.Analytics.Status == "ready"
					}
					if strings.Contains(p.Body, "pito-video-linked-game-card") {
						st.linkedHandle = p.ReplyHandle
					}
				}
				return st
			}
			waitGlance := func(check func(glanceState) bool) (glanceState, bool) {
				deadline := time.Now().Add(45 * time.Second)
				var last glanceState
				for time.Now().Before(deadline) {
					last = fetchGlance()
					if check(last) {
						return last, true
					}
					time.Sleep(1200 * time.Millisecond)
				}
				return last, false
			}

			st, ok := waitGlance(func(s glanceState) bool { return s.ready })
			if !ok {
				t.Fatal("glance never reached ready")
			}
			// The ready body carries the sparkline cells the TUI lifts.
			for _, want := range []string{"pito-analytics-scalars", "pito-metric__row", "scalars__label", "scalars__value"} {
				if !strings.Contains(st.body, want) {
					t.Errorf("glance body missing %q", want)
				}
			}
			if tc.linked && st.linkedHandle == "" {
				t.Error("linked-game card never materialized")
			}

			// analytics_glance replies: analyze appends the full charts.
			before := st.numEvents
			if _, err := client.SendMessage(ctx, uuid, fmt.Sprintf("#%s analyze", st.handle), 1000); err != nil {
				t.Fatalf("glance analyze reply: %v", err)
			}
			if _, ok := waitGlance(func(s glanceState) bool { return s.numEvents > before }); !ok {
				t.Error("glance analyze reply appended nothing")
			}
		})
	}
}
