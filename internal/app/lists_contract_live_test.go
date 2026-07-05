//go:build live

package app

// Live contract spec for the LIST reply surface — every with/without
// kwarg per noun (canonical sets read from pito's list_columns.rb), every
// base sort key both directions, and every sortable column sorted while
// visible (requires_with: the sort vocabulary grows and shrinks with the
// columns). Excluded from CI; run:
//
//	go test -tags live -run TestListReplyContract -v -timeout 900s ./internal/app/
//
// The mutation pipeline is asynchronous server-side (FollowUpDispatchJob →
// event.update! → replace_event), so every assertion polls the backfill.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/config"
)

// Base sort keys per noun, from pito's list_columns.rb: channels sort by
// handle/title only (SORT_KEYS — there is no id key); games and vids carry
// id/title in SORT_SPECS with requires_with: false. Unknown tokens are a
// lenient no-op server-side, so only real keys can be asserted.
var listSortKeys = map[string][]string{
	"channels": {"handle", "title"},
	"games":    {"id", "title"},
	"vids":     {"id", "title"},
}

var listColumns = map[string]map[string]string{ // noun → kwarg → heading
	"channels": {"likes": "Likes"},
	"games": {
		"platform": "Platform", "genre": "Genre", "developer": "Developer",
		"publisher": "Publisher", "channels": "Channels", "footage": "Footage",
		"price": "Price", "views": "Views", "likes": "Likes",
	},
	"vids": {
		"channel": "Channel", "visibility": "Visibility", "game": "Game",
		"duration": "Duration", "views": "Views", "likes": "Likes",
		"category": "Category",
	},
}

// Columns that also join the sort vocabulary while visible
// (requires_with: true in SORT_SPECS / counter sort lambdas). Absent by
// design: games platform (icons don't order, owner G26.7) and vids
// category (not in SORT_SPECS).
var sortableWith = map[string]map[string]bool{
	"channels": {"likes": true},
	"games": {
		"genre": true, "developer": true, "publisher": true, "channels": true,
		"footage": true, "price": true, "views": true, "likes": true,
	},
	"vids": {
		"channel": true, "visibility": true, "game": true,
		"duration": true, "views": true, "likes": true,
	},
}

// Default-visible sortable columns (no `with` needed): channels ship
// subs/views/vids visible (DEFAULT_COLUMNS), each with a sort lambda.
var defaultSortable = map[string]map[string]string{ // noun → token → heading
	"channels": {"subs": "Subs", "views": "Views", "vids": "Vids"},
}

type listState struct {
	eventID  int64
	handle   string
	headings []string
	rows     [][]string
	firstRef string
}

func (s listState) hasHeading(want string) bool {
	return s.headingIndex(want) >= 0
}

func (s listState) headingIndex(want string) int {
	for i, h := range s.headings {
		if strings.EqualFold(strings.TrimSpace(h), want) {
			return i
		}
	}
	return -1
}

// firstCell is row 0's cell under the named heading ("" when absent).
func (s listState) firstCell(heading string) string {
	idx := s.headingIndex(heading)
	if idx < 0 || len(s.rows) == 0 || idx >= len(s.rows[0]) {
		return ""
	}
	return s.rows[0][idx]
}

// distinctInColumn counts distinct cell values under the named heading —
// a column that is uniform on dev cannot show a sort flip.
func (s listState) distinctInColumn(heading string) int {
	idx := s.headingIndex(heading)
	if idx < 0 {
		return 0
	}
	seen := map[string]bool{}
	for _, row := range s.rows {
		if idx < len(row) {
			seen[row[idx]] = true
		}
	}
	return len(seen)
}

func fetchListState(t *testing.T, client *api.Client, uuid string, eventID int64) (listState, bool) {
	t.Helper()
	page, err := client.FetchChat(t.Context(), uuid)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range page.Events {
		var p struct {
			ReplyHandle string `json:"reply_handle"`
			// Heading entries are strings OR {text,...} objects — decode
			// raw and resolve per entry (the renderer's tableCell lesson).
			TableHeading []json.RawMessage `json:"table_heading"`
			TableRows    []struct {
				Cells []struct {
					Text string `json:"text"`
				} `json:"cells"`
			} `json:"table_rows"`
		}
		if json.Unmarshal(ev.Payload, &p) != nil || len(p.TableRows) == 0 {
			continue
		}
		if eventID != 0 && ev.ID != eventID {
			continue
		}
		state := listState{eventID: ev.ID, handle: p.ReplyHandle}
		for _, raw := range p.TableHeading {
			var plain string
			if json.Unmarshal(raw, &plain) == nil {
				state.headings = append(state.headings, plain)
				continue
			}
			var obj struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(raw, &obj) == nil {
				state.headings = append(state.headings, obj.Text)
			}
		}
		for _, row := range p.TableRows {
			cells := make([]string, len(row.Cells))
			for i, cell := range row.Cells {
				cells[i] = strings.TrimSpace(cell.Text)
			}
			state.rows = append(state.rows, cells)
		}
		// First TEXTUAL cell identifies the row (channels lead with an
		// html avatar cell; skip anything tag-shaped or empty).
		for _, text := range state.rows[0] {
			if text != "" && !strings.HasPrefix(text, "<") {
				state.firstRef = text
				break
			}
		}
		return state, true
	}
	return listState{}, false
}

// waitListState polls until check passes or the timeout elapses,
// returning the last fetched state either way.
func waitListState(t *testing.T, client *api.Client, uuid string, eventID int64, timeout time.Duration, check func(listState) bool) (listState, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last listState
	for time.Now().Before(deadline) {
		if state, found := fetchListState(t, client, uuid, eventID); found {
			last = state
			if check(last) {
				return last, true
			}
		}
		time.Sleep(700 * time.Millisecond)
	}
	return last, false
}

func fetchOrFatal(t *testing.T, client *api.Client, uuid string, eventID int64) listState {
	t.Helper()
	state, found := fetchListState(t, client, uuid, eventID)
	if !found {
		t.Fatal("list vanished mid-spec")
	}
	return state
}

func TestListReplyContract(t *testing.T) {
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
	send := func(uuid, input string) {
		t.Helper()
		if _, err := client.SendMessage(ctx, uuid, input, 120); err != nil {
			t.Fatalf("send %q: %v", input, err)
		}
	}

	// testColumnSort asserts the requires_with contract for one visible
	// column: descending then ascending must lead with different values in
	// that column. Columns uniform on dev can't flip — logged, not failed.
	testColumnSort := func(t *testing.T, uuid string, eventID int64, handle, token, heading string) {
		t.Helper()
		before := fetchOrFatal(t, client, uuid, eventID)
		if before.distinctInColumn(heading) < 2 {
			t.Logf("sort %s: %q column uniform on dev — flip not assertable, skipping", token, heading)
			return
		}
		send(uuid, fmt.Sprintf("#%s sort %s desc", handle, token))
		// Tolerant settle: the current order may already be descending.
		descSt, _ := waitListState(t, client, uuid, eventID, 8*time.Second, func(s listState) bool {
			return s.firstCell(heading) != before.firstCell(heading)
		})
		if descSt.eventID == 0 {
			descSt = fetchOrFatal(t, client, uuid, eventID)
		}
		send(uuid, fmt.Sprintf("#%s sort %s", handle, token))
		if _, ok := waitListState(t, client, uuid, eventID, 15*time.Second, func(s listState) bool {
			return s.firstCell(heading) != descSt.firstCell(heading)
		}); !ok {
			t.Errorf("sort %s: ascending never led with a different %s than descending (stuck at %q)",
				token, heading, descSt.firstCell(heading))
		}
	}

	for noun, columns := range listColumns {
		noun, columns := noun, columns
		t.Run(noun, func(t *testing.T) {
			time.Sleep(1500 * time.Millisecond) // stagger nouns; be kind to dev
			res, err := client.SendMessage(ctx, "", "ls "+noun, 1000)
			if err != nil || res.CreatedUUID == "" {
				t.Fatalf("ls %s: %v %+v", noun, err, res)
			}
			uuid := res.CreatedUUID
			state, ok := waitListState(t, client, uuid, 0, 12*time.Second, func(s listState) bool { return s.handle != "" })
			if !ok {
				t.Fatal("list never materialized")
			}
			eventID, handle := state.eventID, state.handle

			for kwarg, heading := range columns {
				send(uuid, fmt.Sprintf("#%s with %s", handle, kwarg))
				if _, ok := waitListState(t, client, uuid, eventID, 12*time.Second, func(s listState) bool { return s.hasHeading(heading) }); !ok {
					t.Errorf("with %s: heading %q never appeared", kwarg, heading)
					continue
				}
				// While the column is visible it joins the sort vocabulary
				// (requires_with) — platform and category never do. NOTE:
				// channel counter sorts fail until pito's channel_list.rb
				// mutate_sort passes selected_columns + columns (verbs.md
				// 1b, owner fixing Rails-side).
				if sortableWith[noun][kwarg] {
					testColumnSort(t, uuid, eventID, handle, kwarg, heading)
				}
				send(uuid, fmt.Sprintf("#%s without %s", handle, kwarg))
				if _, ok := waitListState(t, client, uuid, eventID, 12*time.Second, func(s listState) bool { return !s.hasHeading(heading) }); !ok {
					t.Errorf("without %s: heading %q never left", kwarg, heading)
				}
			}

			// Default-visible sortable columns need no `with` first.
			for token, heading := range defaultSortable[noun] {
				testColumnSort(t, uuid, eventID, handle, token, heading)
			}

			// Base sort keys: for each, ascending vs descending must lead
			// with different rows. Ascending may coincide with the current
			// order (games/vids arrive id-desc), so the asc settle is a
			// tolerant poll; only the asc→desc flip is a hard assertion.
			prev := fetchOrFatal(t, client, uuid, eventID)
			for _, key := range listSortKeys[noun] {
				send(uuid, fmt.Sprintf("#%s sort %s", handle, key))
				asc, _ := waitListState(t, client, uuid, eventID, 8*time.Second, func(s listState) bool { return s.firstRef != prev.firstRef })
				if asc.eventID == 0 {
					asc = fetchOrFatal(t, client, uuid, eventID)
				}
				send(uuid, fmt.Sprintf("#%s sort %s desc", handle, key))
				desc, ok := waitListState(t, client, uuid, eventID, 15*time.Second, func(s listState) bool { return s.firstRef != asc.firstRef })
				if !ok {
					t.Errorf("sort %s desc: first row never moved off %s", key, asc.firstRef)
				}
				prev = desc
			}
		})
	}
}
