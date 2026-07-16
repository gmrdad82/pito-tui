package ui

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gmrdad82/pito-tui/internal/api"
)

// notifChatOnlyMux serves just enough for a WithConversation("u1") model to
// exist — the notifications panel never needs the scrollback fetched, so
// most tests below skip it and go straight from sized() to typing.
func notifMux(notifications http.HandlerFunc) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /notifications.json", notifications)
	return mux
}

func jsonHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// TestNotificationsSlashIntercept covers the client-side grammar: exact
// "/notifications" (case-insensitive, trailing whitespace trimmed) opens
// the panel and never reaches sendCmd; anything else — the bare tool
// verb with no slash, or the slash form carrying args — round-trips to
// POST /chat exactly as before.
func TestNotificationsSlashIntercept(t *testing.T) {
	var posts []string
	mux := notifMux(jsonHandler(`{"rows":[],"next_cursor":null}`))
	mux.HandleFunc("POST /chat", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		posts = append(posts, body.Input)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"uuid":"u1","turn_id":1}`))
	})

	cases := []struct {
		name  string
		input string
		opens bool
	}{
		{"exact", "/notifications", true},
		{"case-insensitive", "/NOTIFICATIONS", true},
		{"mixed case", "/Notifications", true},
		{"trailing whitespace", "/notifications   ", true},
		{"bare tool, no slash", "notifications", false},
		{"slash form with args", "/notifications now", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newTestModel(t, mux, WithConversation("u1"))
			m = sized(m)
			m.input.SetValue(tc.input)
			next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = next.(Model)

			if tc.opens {
				if m.mode != modeNotifications {
					t.Fatalf("input %q: mode = %v, want modeNotifications", tc.input, m.mode)
				}
				if !m.notif.fetching {
					t.Fatalf("input %q: opening must start the first fetch", tc.input)
				}
				if cmd == nil {
					t.Fatalf("input %q: opening must return a command", tc.input)
				}
				if m.input.Value() != "" {
					t.Errorf("input %q: prompt must clear on intercept", tc.input)
				}
			} else {
				if m.mode == modeNotifications {
					t.Fatalf("input %q must NOT open the panel", tc.input)
				}
				if cmd == nil {
					t.Fatalf("input %q must still send to the server", tc.input)
				}
				cmd() // drains the real POST /chat call
			}
		})
	}
	if len(posts) != 2 {
		t.Fatalf("POST /chat calls = %v, want exactly 2 (the non-intercepted cases)", posts)
	}
}

// TestCtrlSlashOpensNotificationsOverlay covers web parity (owner order
// 2026-07-15, 3.0.0 U1.4): ctrl+/ (onChatKey) takes the exact same path
// as the typed /notifications command — modeNotifications, a fresh
// panel, the first-page fetch already in flight. The legacy alias
// ctrl+_ (0x1F, delivered by terminals without kitty disambiguation) is
// covered too since onChatKey wires both to the same case.
func TestCtrlSlashOpensNotificationsOverlay(t *testing.T) {
	for _, combo := range []tea.KeyPressMsg{
		{Code: '/', Mod: tea.ModCtrl},
		{Code: '_', Mod: tea.ModCtrl},
	} {
		m, _ := newTestModel(t, notifMux(jsonHandler(`{"rows":[],"next_cursor":null}`)), WithConversation("u1"))
		m = sized(m)
		next, cmd := m.Update(combo)
		m = next.(Model)
		if m.mode != modeNotifications {
			t.Fatalf("%v: mode = %v, want modeNotifications", combo, m.mode)
		}
		if !m.notif.fetching {
			t.Errorf("%v: opening must start the first fetch", combo)
		}
		if cmd == nil {
			t.Errorf("%v: opening must return a command", combo)
		}
	}
}

// TestStatusBarShowsCtrlSlashHintAtZeroUnread pins the new "ctrl+/ N ⚑"
// status-bar piece (owner order 2026-07-15, 3.0.0 U1.3) for the common
// steady state: authenticated, nothing unread. ANSI-stripped because
// "ctrl+/" and "0 ⚑" are two separate style.Render calls in statusLine —
// the reset/color-start codes between them break a raw substring match.
func TestStatusBarShowsCtrlSlashHintAtZeroUnread(t *testing.T) {
	m, _ := newTestModel(t, http.NotFoundHandler(), WithConversation("u-1"))
	m = sized(m)
	if view := ansi.Strip(m.viewContent()); !strings.Contains(view, "ctrl+/ 0 ⚑") {
		t.Errorf("status bar must show ctrl+/ 0 ⚑ when authenticated with no unread:\n%s", view)
	}
}

// TestNotificationsPaginationAndDuplicateGuard drives the full two-page
// flow: page 1 lands short of the cursor reaching bottom (no eager
// fetch), scrolling to the last loaded row fires page 2 exactly once
// even under a repeated nudge, and exhaustion (next_cursor null) stops
// further fetches for good.
func TestNotificationsPaginationAndDuplicateGuard(t *testing.T) {
	var calls []string
	mux := notifMux(func(w http.ResponseWriter, r *http.Request) {
		before := r.URL.Query().Get("after")
		calls = append(calls, before)
		switch before {
		case "":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rows":[
				{"id":1,"message":"first",  "read":false,"created_at":"2026-07-04T11:00:00Z"},
				{"id":2,"message":"second", "read":true, "created_at":"2026-07-03T11:00:00Z"}
			],"next_cursor":"c2"}`))
		case "c2":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rows":[
				{"id":3,"message":"third","read":false,"created_at":"2026-07-02T11:00:00Z"}
			],"next_cursor":null}`))
		default:
			t.Errorf("unexpected fetch with before=%q", before)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rows":[],"next_cursor":null}`))
		}
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)

	m.input.SetValue("/notifications")
	m, cmd := driveCmd(m, key("enter"))
	if m.mode != modeNotifications || cmd == nil {
		t.Fatal("enter must open the panel and schedule page 1")
	}

	// Land page 1 by hand (bypassing the batch — see runCmd's sibling
	// pattern elsewhere in this suite for why: only the fetch matters
	// here, not the animation tick riding alongside it).
	m = drive(m, m.notificationsFetchCmd(m.notif.next)())
	if len(m.notif.rows) != 2 || m.notif.next != "c2" || m.notif.fetching {
		t.Fatalf("page 1 not absorbed: rows=%d next=%q fetching=%v",
			len(m.notif.rows), m.notif.next, m.notif.fetching)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want exactly 1 after page 1 (cursor hasn't reached the last row)", calls)
	}

	// Cursor still at row 0 of 2 — not the last row yet, so landing
	// alone must not have triggered page 2.
	if m.notif.fetching {
		t.Fatal("must not eagerly fetch before the cursor reaches the last loaded row")
	}

	// "j" reaches the last loaded row (index 1) and fires page 2.
	m, cmd = driveCmd(m, key("j"))
	if m.notif.cursor != 1 || !m.notif.fetching || cmd == nil {
		t.Fatalf("moving to the last row must trigger page 2: cursor=%d fetching=%v cmd=%v",
			m.notif.cursor, m.notif.fetching, cmd)
	}

	// A duplicate nudge while the fetch is in flight must NOT re-request.
	m, cmd = driveCmd(m, key("j"))
	if cmd != nil {
		t.Fatal("a nudge while fetching must be a no-op (duplicate-fetch guard)")
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want still 1 — the guard must have blocked the duplicate", calls)
	}

	// Land page 2.
	m = drive(m, m.notificationsFetchCmd(m.notif.next)())
	if len(m.notif.rows) != 3 || m.notif.next != "" || !m.notif.fetched || m.notif.fetching {
		t.Fatalf("page 2 not absorbed: rows=%d next=%q fetched=%v fetching=%v",
			len(m.notif.rows), m.notif.next, m.notif.fetched, m.notif.fetching)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want exactly 2 total", calls)
	}

	// Scrolling to the new last row (now exhausted) must not fetch again.
	// (A cmd may still come back — arrival on an unread row PATCHes it
	// read since 2026-07-12 — so the guard is the fetching flag + the
	// call count, not cmd-nilness.)
	m, _ = driveCmd(m, key("j"))
	if m.notif.fetching {
		t.Fatal("an exhausted list must never fetch again")
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want still 2 after exhaustion", calls)
	}
}

// Multiline server messages (the IGDB sync lists its games one per
// line) must collapse to ONE row with the stamp on the right edge —
// owner screenshot 2026-07-12: rows spilled and stamps scattered.
func TestNotificationRowCollapsesMultilineMessages(t *testing.T) {
	row := api.NotificationRow{
		Message: "IGDB nightly sync: checked 66 upcoming game(s)\n\nTekken 7\nProject Motor Racing\nR-Type Dimensions",
		Read:    true,
	}
	line := notificationRowLine(row, 100, time.Now(), false)
	if strings.Contains(ansi.Strip(line), "\n") {
		t.Fatalf("row must be a single line, got %q", ansi.Strip(line))
	}
	if !strings.Contains(ansi.Strip(line), "Tekken 7 Project Motor Racing") {
		t.Fatalf("newlines must collapse to spaces, got %q", ansi.Strip(line))
	}
}

// TestNotificationsUnreadReadGlyphs checks the row glyph/order/stamp
// contract: unread rows wear "● ", read rows wear "○ ", newest-first as
// served (no client resort), stamps day-aware.
func TestNotificationsUnreadReadGlyphs(t *testing.T) {
	mux := notifMux(jsonHandler(`{"rows":[
		{"id":1,"message":"Hades II unlinked from vid 12","read":false,"created_at":"2026-07-04T11:00:00Z"},
		{"id":2,"message":"channel sync finished",          "read":true, "created_at":"2026-06-01T09:00:00Z"}
	],"next_cursor":null}`))
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)

	m.input.SetValue("/notifications")
	m, _ = driveCmd(m, key("enter"))
	m = drive(m, m.notificationsFetchCmd(m.notif.next)())
	m = driveAnim(t, m, 40) // let the slide-up settle before reading the view

	view := ansi.Strip(m.viewContent())
	unreadLine := "● Hades II unlinked from vid 12"
	readLine := "○ channel sync finished"
	ui := strings.Index(view, unreadLine)
	ri := strings.Index(view, readLine)
	if ui < 0 {
		t.Fatalf("unread row missing:\n%s", view)
	}
	if ri < 0 {
		t.Fatalf("read row missing:\n%s", view)
	}
	if ui > ri {
		t.Errorf("rows must render in served (newest-first) order; unread @%d after read @%d", ui, ri)
	}
	if !strings.Contains(view, "11:00") {
		t.Errorf("today's stamp missing:\n%s", view)
	}
	if !strings.Contains(view, "1 Jun 09:00") {
		t.Errorf("same-year day-aware stamp missing:\n%s", view)
	}
}

// TestNotificationsEscAndQClose confirms both close keys start the
// slide-out (mode stays modeNotifications until the closing spring
// settles — spring.go's stepOverlays) and, once settled, land back in
// chat with the panel reset so a later reopen starts clean.
func TestNotificationsEscAndQClose(t *testing.T) {
	for _, closeKey := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, key("q")} {
		mux := notifMux(jsonHandler(`{"rows":[],"next_cursor":null}`))
		m, _ := newTestModel(t, mux, WithConversation("u1"))
		m = sized(m)

		m.input.SetValue("/notifications")
		m, _ = driveCmd(m, key("enter"))
		if m.mode != modeNotifications {
			t.Fatal("setup: panel did not open")
		}
		// Land the first page by hand (driveAnim's gate stays open while
		// a fetch is in flight — its loadingDots rides the same shared
		// phase — so the fetch must resolve before the panel can settle).
		m = drive(m, m.notificationsFetchCmd(m.notif.next)())
		m = driveAnim(t, m, 40) // let the open spring settle first

		// Drawer springs purged 2026-07-12: the close is immediate.
		m, _ = driveCmd(m, closeKey)
		if m.mode != modeChat {
			t.Errorf("%v: mode = %v, want modeChat", closeKey, m.mode)
		}
		if m.notif.fetching || len(m.notif.rows) != 0 {
			t.Errorf("%v: closing must reset the panel, got %+v", closeKey, m.notif)
		}
	}
}

// TestNotificationsUnavailableClosesAndNotices covers the version-gate
// path: api.ErrNotificationsUnavailable (404 — an older pito) must close
// the panel and surface the existing Notice line, not a dead overlay.
func TestNotificationsUnavailableClosesAndNotices(t *testing.T) {
	mux := notifMux(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)

	m.input.SetValue("/notifications")
	m, _ = driveCmd(m, key("enter"))
	m = drive(m, m.notificationsFetchCmd(m.notif.next)())

	if m.mode != modeChat {
		t.Fatalf("unavailable must close the panel back to chat, mode = %v", m.mode)
	}
	want := "notifications need a newer pito — update the server"
	if len(m.notices) == 0 || m.notices[len(m.notices)-1] != want {
		t.Fatalf("notices = %v, want last = %q", m.notices, want)
	}
	if view := m.viewContent(); !strings.Contains(view, want) {
		t.Errorf("notice not rendered in the chat view:\n%s", view)
	}
}

// TestNotificationsGenericErrorShowsDimRowAndRetriesOnNudge covers the
// non-unavailable failure path: the panel stays open with a dim error
// row, and there is no automatic retry loop — only a manual scroll
// nudge (any movement key) tries again.
func TestNotificationsGenericErrorShowsDimRowAndRetriesOnNudge(t *testing.T) {
	calls := 0
	mux := notifMux(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rows":[{"id":1,"message":"ok now","read":false,"created_at":"2026-07-04T11:00:00Z"}],"next_cursor":null}`))
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m)

	m.input.SetValue("/notifications")
	m, _ = driveCmd(m, key("enter"))
	m = drive(m, m.notificationsFetchCmd(m.notif.next)())
	m = driveAnim(t, m, 40) // let the slide-up settle before reading the view

	if m.mode != modeNotifications {
		t.Fatal("a generic fetch error must not close the panel")
	}
	if m.notif.err == "" {
		t.Fatal("expected a dim error message recorded on the panel")
	}
	if view := m.viewContent(); !strings.Contains(view, m.notif.err) {
		t.Errorf("error text missing from the panel view:\n%s", view)
	}
	if calls != 1 {
		t.Fatalf("must not auto-retry: calls = %d, want 1", calls)
	}

	// A manual scroll nudge (rows are empty, so "j" doesn't move the
	// cursor — but it still re-runs the fetch trigger) retries.
	m, cmd := driveCmd(m, key("j"))
	if cmd == nil {
		t.Fatal("a nudge after a failed fetch must retry")
	}
	m = drive(m, m.notificationsFetchCmd(m.notif.next)())
	if len(m.notif.rows) != 1 || m.notif.err != "" || m.notif.fetching {
		t.Fatalf("retry did not land: rows=%d err=%q fetching=%v", len(m.notif.rows), m.notif.err, m.notif.fetching)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 after the retry", calls)
	}
}

// TestNotificationsPageKeysMoveAndClampCursor exercises pgup/pgdn (the
// picker itself only wires j/k/up/down — this panel additionally
// supports paging per spec) and confirms the cursor clamps at both ends.
func TestNotificationsPageKeysMoveAndClampCursor(t *testing.T) {
	var rows strings.Builder
	for i := 1; i <= 10; i++ {
		if i > 1 {
			rows.WriteString(",")
		}
		rows.WriteString(`{"id":` + itoa(i) + `,"message":"row","read":true,"created_at":"2026-07-04T11:00:00Z"}`)
	}
	mux := notifMux(jsonHandler(`{"rows":[` + rows.String() + `],"next_cursor":null}`))
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = sized(m) // 80x24 → notifPageStep(24) = 20

	m.input.SetValue("/notifications")
	m, _ = driveCmd(m, key("enter"))
	m = drive(m, m.notificationsFetchCmd(m.notif.next)())
	if len(m.notif.rows) != 10 {
		t.Fatalf("setup: rows = %d, want 10", len(m.notif.rows))
	}

	m = drive(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.notif.cursor != 9 {
		t.Errorf("pgdown must clamp to the last row: cursor = %d, want 9", m.notif.cursor)
	}
	m = drive(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.notif.cursor != 0 {
		t.Errorf("pgup must clamp to the first row: cursor = %d, want 0", m.notif.cursor)
	}
}

// TestNotificationStampDayAwareFormat pins the row stamp to the EXACT
// same layout rules as render.(*R).stamp (today / same year / other
// year) — the spec requires the formats stay identical between panels.
func TestNotificationStampDayAwareFormat(t *testing.T) {
	now := fixedNow
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"today", now, now.Format("15:04")},
		{"same year", now.AddDate(0, 0, -5), now.AddDate(0, 0, -5).Format("2 Jan 15:04")},
		{"other year", now.AddDate(-1, 0, 0), now.AddDate(-1, 0, 0).Format("2 Jan '06 15:04")},
		{"zero time", time.Time{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := notificationStamp(tc.at, now); got != tc.want {
				t.Errorf("notificationStamp(%v, %v) = %q, want %q", tc.at, now, got, tc.want)
			}
		})
	}
}

func TestTruncateEllipsis(t *testing.T) {
	cases := []struct {
		s     string
		width int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello w…"},
		{"x", 0, ""},
		{"hi", 1, "…"},
	}
	for _, tc := range cases {
		if got := truncateEllipsis(tc.s, tc.width); got != tc.want {
			t.Errorf("truncateEllipsis(%q, %d) = %q, want %q", tc.s, tc.width, got, tc.want)
		}
	}
}

// TestNotificationRowSelectionWearsTheCursorStripe pins the 2026-07-13
// ruling (supersedes the 07-12 elevated-gray retint): the selected
// modal row wears the ▌ accent bar + the plum ColorZebra stripe — one
// cursor language across /resume, /notifications, ctrl+k and the
// entity pickers (picker.go cursorStripe).
func TestNotificationRowSelectionWearsTheCursorStripe(t *testing.T) {
	row := api.NotificationRow{ID: 1, Message: "you have a new reply", Read: false, CreatedAt: fixedNow}
	out := notificationRowLine(row, 60, fixedNow, true)
	if !strings.Contains(out, "▌") {
		t.Errorf("selected row must lead with the cursor bar:\n%q", out)
	}
	if !strings.Contains(out, "48;2;") {
		t.Errorf("selected row must wear the zebra stripe background:\n%q", out)
	}
	unselected := notificationRowLine(row, 60, fixedNow, false)
	if strings.Contains(unselected, "48;2;") {
		t.Errorf("unselected notification row must carry no background:\n%q", unselected)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func TestNotificationRowsFlattenInlineHTML(t *testing.T) {
	// W7.f audit finding: server messages can carry <strong> tags —
	// they must flatten, never render literally.
	got := notificationRowLine(api.NotificationRow{
		ID: 1, Message: "<strong>Elden Ring</strong> synced", Read: false,
	}, 60, fixedNow, false)
	if strings.Contains(got, "<strong>") || !strings.Contains(got, "Elden Ring synced") {
		t.Errorf("tags leaked or text lost: %q", got)
	}
}

// TestViewportRowsFloorsAndPassesThrough pins viewportRows' own contract
// (owner 2026-07-15, viewport-driven paging): a tiny pane floors at 10
// rather than pulling single-row pages, while anything at or above the
// floor passes through untouched.
func TestViewportRowsFloorsAndPassesThrough(t *testing.T) {
	if got := viewportRows(3); got != 10 {
		t.Errorf("viewportRows(3) = %d, want 10 (floor)", got)
	}
	if got := viewportRows(24); got != 24 {
		t.Errorf("viewportRows(24) = %d, want 24 (passthrough)", got)
	}
}

// TestNotificationsFetchSendsViewportDerivedLimit pins viewportRows'
// composition at the notifications call site (notificationsFetchCmd): the
// outgoing GET /notifications.json limit is viewportRows(notifPageStep(m.height)),
// not a fixed page size.
func TestNotificationsFetchSendsViewportDerivedLimit(t *testing.T) {
	var gotLimit string
	mux := notifMux(func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rows":[],"next_cursor":null}`))
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 30})

	m = drive(m, m.notificationsFetchCmd(m.notif.next)())

	want := itoa(viewportRows(notifPageStep(m.height)))
	if gotLimit != want {
		t.Errorf("limit = %q, want %q (viewportRows(notifPageStep(%d)))", gotLimit, want, m.height)
	}
}

// TestNotificationsResizeBetweenPagesChangesLimit covers viewport-driven
// paging across a resize (owner 2026-07-15): the panel loads page 1 at
// one terminal height, the owner resizes before scrolling to the last
// row, and the page-2 fetch this triggers must carry the NEW height's
// limit. Cursor continuity (the after= param) is already pinned by
// TestNotificationsPaginationAndDuplicateGuard — this test only cares
// that the limit tracks the live viewport across the resize.
func TestNotificationsResizeBetweenPagesChangesLimit(t *testing.T) {
	var limits []string
	mux := notifMux(func(w http.ResponseWriter, r *http.Request) {
		limits = append(limits, r.URL.Query().Get("limit"))
		before := r.URL.Query().Get("after")
		w.Header().Set("Content-Type", "application/json")
		if before == "" {
			_, _ = w.Write([]byte(`{"rows":[
				{"id":1,"message":"first", "read":false,"created_at":"2026-07-04T11:00:00Z"},
				{"id":2,"message":"second","read":true, "created_at":"2026-07-03T11:00:00Z"}
			],"next_cursor":"c2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"rows":[
			{"id":3,"message":"third","read":false,"created_at":"2026-07-02T11:00:00Z"}
		],"next_cursor":null}`))
	})
	m, _ := newTestModel(t, mux, WithConversation("u1"))
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 30})

	m.input.SetValue("/notifications")
	m, cmd := driveCmd(m, key("enter"))
	if m.mode != modeNotifications || cmd == nil {
		t.Fatal("enter must open the panel and schedule page 1")
	}
	m = drive(m, m.notificationsFetchCmd(m.notif.next)())
	if len(m.notif.rows) != 2 {
		t.Fatalf("page 1 not absorbed: rows=%d", len(m.notif.rows))
	}

	// Resize BEFORE the page-2 trigger — the new height must own page 2's limit.
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 12})

	m, cmd = driveCmd(m, key("j")) // reaches the last loaded row, fires page 2
	if !m.notif.fetching || cmd == nil {
		t.Fatal("moving to the last row after a resize must still trigger page 2")
	}
	m = drive(m, m.notificationsFetchCmd(m.notif.next)())
	if len(m.notif.rows) != 3 {
		t.Fatalf("page 2 not absorbed: rows=%d", len(m.notif.rows))
	}

	if len(limits) != 2 {
		t.Fatalf("limits captured = %v, want 2 requests", limits)
	}
	wantFirst := itoa(viewportRows(notifPageStep(30)))
	wantSecond := itoa(viewportRows(notifPageStep(12)))
	if limits[0] != wantFirst {
		t.Errorf("page 1 limit = %q, want %q (height 30)", limits[0], wantFirst)
	}
	if limits[1] != wantSecond {
		t.Errorf("page 2 limit = %q, want %q (height 12 after resize)", limits[1], wantSecond)
	}
	if limits[0] == limits[1] {
		t.Fatal("resize must actually change the requested limit between pages")
	}
}
