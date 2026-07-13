// Package ui is the Bubble Tea program: one model covering the
// conversation picker and the chat screen. It mirrors the web shell —
// scrollback of turn blocks, one prompt, slash commands passed through as
// raw text. The server grammar stays authoritative; nothing is parsed here.
package ui

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
)

type mode int

const (
	modePicker mode = iota
	modeChat
	modeNotifications
	modeCommandPalette
	modeEntityPicker
	modeAiPicker
	// modeFootage covers the ctrl+f footage flow's folder + probing steps
	// (footage.go) — the game-picking step rides modeEntityPicker with
	// entityPicker.footage=true instead of a step of its own, so
	// entitypicker.go's fetch/filter/render machinery drives it unchanged.
	modeFootage
	// modeWarn is the generic full-screen warning/error overlay (footage.go's
	// ffprobe-missing gate is its only caller today) — any key dismisses.
	modeWarn
)

const maxNotices = 3

// ConnectFunc starts the cable subscription for a conversation. The app
// layer supplies it; tests record it.
type ConnectFunc func(uuid string)

// Notifier plays UI sounds; the zero implementation is silence.
type Notifier interface {
	Send()
	Receive()
}

type noopNotifier struct{}

func (noopNotifier) Send()    {}
func (noopNotifier) Receive() {}

type Model struct {
	client  *api.Client
	connect ConnectFunc
	sounds  Notifier

	mode         mode
	plainRender  bool
	glamourStyle string
	truecolor    bool
	renderer     *render.R
	shimmer      map[int64]bool // turns with shimmer-marked words (animate forever)
	// revealing tracks freshly-arrived events whose charts/bars grow in
	// (the web's pito-bar-reveal): eventID → one harmonica spring state
	// (position + velocity), replacing a birth-time + fixed-duration ease
	// — see spring.go's revealSpringPhysics.
	phase     float64
	animating bool
	// aliveTicks counts real (gate-open) onAnimTick ticks — the shimmer
	// conductor's "30s of ALIVE time" clock (ambient.go's conductorWeight),
	// deliberately NOT wall-clock idle time.
	aliveTicks int64
	// dotPulseTicks: the presence dot's one green breath per delivered
	// cable message (ambient.go effect 3; owner 2026-07-12: no idle
	// breathing — activity only).
	dotPulseTicks int64
	// quitArmed: ticks left in the press-ctrl+c-again-to-quit window.
	quitArmed int64
	// scrollEasing: a follow-mode glide toward the bottom is in flight
	// (easeTowardBottom) — holds the animation gate open until it lands.
	scrollEasing bool
	// skyPhase/skyTicking: the star sky's own slow drift (ambient.go) —
	// deliberately OFF the fast gate so it breathes even at rest. skyOn
	// is WithStarSky's setting (NewModel defaults it true; the test
	// harness forces false — braille stars read as thumb/chart glyphs to
	// frame-scanning assertions).
	skyOn      bool
	skyPhase   float64
	skyTicking bool
	// rippleTick/rippleAnim drive the status-bar ripple's own 600ms
	// window (ripple.go effect 1) — see beginRipple/rippleActive.
	rippleTick int64
	rippleAnim bool
	// unreadFrom/unreadOdoTick/unreadOdoAnim drive the ✉ badge's roll-to-
	// new-value animation (ripple.go effect 2) — see beginUnreadRoll/
	// displayUnread. unread itself (below, chat state) stays the settled
	// TARGET throughout; these three are purely the in-flight display.
	unreadFrom    int
	unreadOdoTick int64
	unreadOdoAnim bool
	// shaking drives the ambassador wave's error-shake jitter (micro.go
	// effect 1) — eventID → its in-flight shake state. See
	// beginErrorShake/pushShakeOffsets.
	shaking map[int64]errorShake
	// ghostTarget/ghostTick/ghostTyping drive the palette's ghost-hint
	// type-in (micro.go effect 3) — see beginGhostType/ghostDisplay.
	ghostTarget string
	ghostTick   int64
	ghostTyping bool
	// thumbFadeTick/thumbFading drive the scroll-thumb's fade-out once
	// the viewport lands back at the bottom (micro.go effect 4) — see
	// setFollow.
	thumbFadeTick int64
	thumbFading   bool

	// splashOn/splash drive the startup wordmark (chrome.go/splash.go
	// effect 1) — splashOn is WithSplash's own setting (NewModel defaults
	// it true; the test harness forces it false — see model_test.go's
	// newTestModel), splash is the splash's own hold/rise state machine.
	splashOn bool
	splash   splashState
	// footerAnim drives the '?' keymap footer's open/close spring
	// (chrome.go effect 2) — a plain spring.go overlayAnim value this
	// file steps directly; no changes to spring.go were needed.
	footerAnim overlayAnim

	// tour drives --tour's self-playing walkthrough (tour.go): a script
	// of {caption, command, dwell} steps that types each command into
	// the real input tick by tick, submits it through the ordinary
	// Enter path (no parallel send code), and holds for its dwell before
	// advancing — see WithTour/stepTour. A zero value (empty script)
	// means "no tour," everywhere this is read (tourActive).
	tour tourState

	// closeAction fires once the relevant overlay's closing spring
	// settles at 0 — see spring.go's stepOverlays: the mode field itself
	// doesn't switch until here ("closing completes before the mode
	// actually switches back").

	// picker state
	rows   []pickerRow
	cursor int
	// pickerAnim is the picker overlay's open/close spring (spring.go).
	now func() time.Time
	// pickerNext/pickerFetching are the picker's pagination follow-on
	// state (tui-needs ask 9a): pickerNext is the cursor for the picker's
	// NEXT fetch ("" means exhausted — either the list truly ended, or an
	// old server never sent next_cursor at all, indistinguishable by
	// design, see ResumeList's doc comment); pickerFetching guards
	// against firing a duplicate request while one is already in flight.
	// Mirrors notificationsPanel's next/fetching (notifications.go).
	pickerNext     string
	pickerFetching bool
	// pickerRenaming is the uuid of the row currently in inline-rename
	// mode ("" when none) — the picker's `n` key (mirrors the web's
	// pito--rename inline <input> swap, resume_controller.js "n" handler).
	// pickerRenameInput is the textinput.Model backing it, built fresh on
	// every rename start (see openPickerRename).
	pickerRenaming    string
	pickerRenameInput textinput.Model
	// pickerDeleteArmed: ticks left in the picker's dd delete-arm window
	// ("first d arms the highlighted row, second d within the window
	// deletes it" — resume_controller.js's #arm/#disarm, 500ms). Always
	// scoped to the row currently under the cursor: moving the highlight
	// disarms, exactly like the web.
	pickerDeleteArmed int64

	// notifications overlay state (see notifications.go)
	notif notificationsPanel
	// notifAnim is the notifications overlay's open/close spring (spring.go).

	// chat state
	sc            chatScroller
	scTail        []string // comet/notices window tail (scroll.go)
	input         textinput.Model
	spin          spinner.Model
	transcript    *Transcript
	conv          api.Conversation
	conn          cable.ConnState
	sawDisconnect bool
	cableStarted  bool
	pending       map[int64]bool
	follow        bool
	notices       []string
	needsLogin    bool
	showHelp      bool
	syntheticID   int64 // negative-ID counter for local echo events
	suggest       *api.Suggestions
	suggestSeq    int
	suggestSel    int
	// Input history (owner 2026-07-12: "In web I can do up/down arrows to
	// navigate through conversation chat box history. Implement this also
	// here."). Parity spec: pito's history_controller.js — oh-my-zsh
	// PREFIX recall: the first ↑ snapshots the buffer as the search
	// prefix and filters histEntries to those that start with it; ↑ walks
	// older matches, ↓ newer, index -1 restores the snapshot draft, no
	// wrap at either end; any real edit ends the recall session. Entries
	// are newest-first, consecutive-dupes collapsed, capped at 50 — seeded
	// from the transcript's echo events (the web seeds from the
	// conversation's last 50 turns) and prepended on every send.
	histEntries []string
	histIndex   int      // -1 = at the snapshot draft
	histDraft   string   // buffer text when the recall session began
	histPrefix  *string  // nil = no active recall session
	histMatches []string // histEntries filtered by prefix, newest first
	// ctrl+k command palette (ctrlk.go, owner 2026-07-12).
	ctrlK ctrlKPanel
	// show game / show vid picker (entitypicker.go, owner 2026-07-12).
	entity entityPicker
	// /config ai model picker (aipicker.go, owner 2026-07-12).
	aiPicker aiPickerPanel
	// ctrl+f "update footage" flow (footage.go, owner 2026-07-13): folder +
	// probing state, the exec seam for ffprobe, and the persisted last-used
	// folder (WithFootageFolder — the app layer resolves/saves the disk
	// file, this is just the in-memory value + callback).
	footage           footageFlow
	footageExec       footageExec
	footageFolder     string
	saveFootageFolder func(string) error
	// warn is the generic full-screen warning/error overlay's state
	// (footage.go's ffprobe-missing gate today).
	warn warnPanel
	// Mouse text selection + copied-toast (select.go, owner 2026-07-12).
	selecting              bool
	selAnchorX, selAnchorY int
	selCursorX, selCursorY int
	toastText              string
	toastTicks             int64
	loadErr                string
	pickerNotice           string
	meterCtx               *api.ContextMeter // server-computed context (render only)
	// serverTag is the SERVER build's identity for the mini status —
	// pito's fat-cut 2026-07-12: the status reads "dot %{tag}", the
	// nickname/"me" concept is gone. dev.pitomd.com/localhost read as
	// the literal "dev" without asking; other hosts fetch GET /version
	// (the web's cable-health endpoint) after auth and on reconnect.
	serverTag string
	unread    int
	// Scope cyclers (web parity): shift+tab cycles the channel while a
	// `list vids/games` is typed; ctrl+space cycles the period during
	// `analyze` (terminals cannot see shift+space — documented stand-in).
	scopeChannel string
	scopePeriod  string
	channels     []string // ["@all", …] — served by chat.json when the ask lands

	width, height int
	ready         bool
}

// Option configures the model.
type Option func(*Model)

// WithConversation opens a conversation directly, skipping the picker.
func WithConversation(uuid string) Option {
	return func(m *Model) {
		m.mode = modeChat
		m.conv.UUID = uuid
	}
}

// WithStarSky toggles the ambient star sky (tests force it off — its
// braille stars would photobomb frame-scanning assertions).
func WithStarSky(on bool) Option {
	return func(m *Model) { m.skyOn = on }
}

// WithNewConversation opens an empty chat with no uuid: the first send
// creates the conversation server-side ({uuid} 201 reply).
func WithNewConversation() Option {
	return func(m *Model) { m.mode = modeChat }
}

// WithLoginRequired opens an unauthenticated chat: the resume picker is
// unavailable (it would 401), so the user lands in a fresh conversation
// with a banner saying to send /login <code> — the server grammar mints
// the session and the reply cookie lands in the jar automatically.
func WithLoginRequired() Option {
	return func(m *Model) {
		m.mode = modeChat
		m.needsLogin = true
	}
}

// WithPlainRender disables glamour/color variance for golden tests.
func WithPlainRender() Option {
	return func(m *Model) { m.plainRender = true }
}

// WithGlamourStyle sets the markdown style ("dark"/"light"), resolved by
// the app BEFORE the program starts (querying inside the TUI deadlocks).
func WithGlamourStyle(style string) Option {
	return func(m *Model) { m.glamourStyle = style }
}

// WithNow pins the clock (relative timestamps in golden tests).
func WithNow(now func() time.Time) Option {
	return func(m *Model) { m.now = now }
}

// WithSounds wires the sound player.
func WithSounds(n Notifier) Option {
	return func(m *Model) {
		if n != nil {
			m.sounds = n
		}
	}
}

// WithTruecolor turns on gradient shimmer (COLORTERM detection happens
// in the app layer, before Bubble Tea owns the terminal).
func WithTruecolor(on bool) Option {
	return func(m *Model) { m.truecolor = on }
}

// WithSplash toggles the startup wordmark (chrome.go/splash.go effect 1).
// NewModel's own struct literal defaults this true; WithSplash(false) is
// the seam the test harness (model_test.go's newTestModel) uses to keep it
// out of every test's very first frame — the same always-prepended shape
// WithPlainRender already uses for golden determinism. A caller-supplied
// WithSplash after that prepended default still wins (Option application
// is plain in-order last-writer-wins), so an individual test can re-arm it
// to exercise the splash itself.
func WithSplash(on bool) Option {
	return func(m *Model) { m.splashOn = on }
}

// WithTour arms `--tour` (tour.go): a brand-new conversation — the same
// blank-uuid path WithNewConversation opens, the first send creates it
// server-side — that plays script end to end with zero interaction once
// the splash clears. An empty script is a no-op, so a caller can pass
// TourScript's result straight through without checking it first.
func WithTour(script []tourStep) Option {
	return func(m *Model) {
		if len(script) == 0 {
			return
		}
		m.mode = modeChat
		m.tour = tourState{script: script, active: true, caption: script[0].caption}
	}
}

// The prompt's two moods: the user accent, and the AI accent while an
// @ai turn is being typed (2.0.0 — the web tints its chatbox bar on the
// same /^\s*@ai\b/i pattern).
var (
	aiInputRe          = regexp.MustCompile(`(?i)^\s*@ai\b`)
	defaultPromptStyle = lipgloss.NewStyle().Foreground(render.ColorAccent).Bold(true)
	aiPromptStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#5170ff")).Bold(true)
)

// widthCapEnabled gates the 100-column containment law. OFF per the
// owner's 2.0.0 smoke ruling — full terminal width everywhere; flip to
// true to restore the contained column (+ star-field margin).
const widthCapEnabled = false

// replyPrefill is Shift+R's fill for one live handle — ai messages
// continue their thread with "@ai " (2.0.0), everything else replies bare.
func replyPrefill(h LiveHandle) string {
	if h.Kind == api.KindAi {
		return "#" + h.Handle + " @ai "
	}
	return "#" + h.Handle + " "
}

func NewModel(client *api.Client, connect ConnectFunc, opts ...Option) Model {
	input := textinput.New()
	input.Placeholder = "/help to see available commands"
	input.Prompt = "> "
	istyles := input.Styles()
	istyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(render.ColorFaint)
	istyles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(render.ColorFaint)
	istyles.Focused.Prompt = defaultPromptStyle
	istyles.Blurred.Prompt = defaultPromptStyle
	istyles.Cursor.Color = render.ColorAccent
	input.SetStyles(istyles)
	input.Focus()

	// The web's post-command comet (shell/post_command_dots): a bright
	// head sweeping across a trail of dots.
	comet := spinner.Spinner{
		Frames: []string{
			"●∙∙∙∙∙∙∙", "∙●∙∙∙∙∙∙", "∙∙●∙∙∙∙∙", "∙∙∙●∙∙∙∙",
			"∙∙∙∙●∙∙∙", "∙∙∙∙∙●∙∙", "∙∙∙∙∙∙●∙", "∙∙∙∙∙∙∙●",
		},
		FPS: time.Second / 10,
	}
	spin := spinner.New(
		spinner.WithSpinner(comet),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(render.ColorAccent)),
	)

	m := Model{
		client:       client,
		connect:      connect,
		sounds:       noopNotifier{},
		mode:         modePicker,
		input:        input,
		spin:         spin,
		pending:      map[int64]bool{},
		shimmer:      map[int64]bool{},
		shaking:      map[int64]errorShake{},
		follow:       true,
		histIndex:    -1, // -1 = "at the draft" (history_controller.js connect())
		now:          time.Now,
		scopeChannel: "@all",
		scopePeriod:  "7d",
		splashOn:     true,
		skyOn:        true,
		footageExec:  realFootageExec{},
	}
	m.transcript = NewTranscript(nil)
	for _, opt := range opts {
		opt(&m)
	}
	if cometTailEnabled && m.truecolor {
		// ripple.go effect 4: swap the plain comet for the pre-styled
		// truecolor variant now that WithTruecolor has landed (opts run
		// AFTER spin is built above) — Frames are fixed at spinner
		// construction, so this is the one seam that has to live here
		// rather than at render time. Style resets to the zero value:
		// cometFrames' cells are already fully colored, and leaving the
		// old ColorAccent wrap in place would flatten the tail's own
		// gradient back to one flat hue on every render.
		m.spin.Spinner.Frames = cometFrames()
		m.spin.Style = lipgloss.NewStyle()
	}
	return m
}

// ResumeHint exposes the active conversation's identity for the app
// layer's post-quit "resume this conversation" hint (owner 2026-07-14,
// "like claude does"): app.Run prints it once program.Run() returns —
// Bubble Tea owns the terminal until then, so nothing in this package ever
// prints it itself. ok is false when there is nothing to resume — a
// brand-new conversation (WithNewConversation/the default boot) that never
// got its first send still carries a blank uuid, and the unauthenticated
// /login banner (WithLoginRequired) never sets one either.
func (m Model) ResumeHint() (uuid, label string, ok bool) {
	if m.conv.UUID == "" {
		return "", "", false
	}
	return m.conv.UUID, m.conv.Label(), true
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	switch {
	case m.mode == modeChat && m.conv.UUID != "":
		cmds = append(cmds, m.fetchChatCmd(m.conv.UUID, false))
	case m.mode == modeChat:
		// Fresh-chat boot (pito's flow): no scrollback to fetch, but the
		// ✉ unread count must not sit empty until the first send —
		// resume.json page 1 carries it (owner 2026-07-12).
		cmds = append(cmds, m.fetchResumeCmd())
	case m.mode == modePicker:
		cmds = append(cmds, m.fetchResumeCmd())
	}
	return tea.Batch(cmds...)
}

// ── Commands ────────────────────────────────────────────────────────────

func (m Model) fetchChatCmd(uuid string, resync bool) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		page, err := client.FetchChat(context.Background(), uuid)
		return ChatFetchedMsg{Page: page, Resync: resync, Err: err}
	}
}

func (m Model) fetchResumeCmd() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		list, err := client.FetchResume(context.Background(), "", 0)
		return ResumeFetchedMsg{List: list, Err: err}
	}
}

// fetchResumeMoreCmd GETs the picker's next page (tui-needs ask 9a) at
// the given cursor. Mirrors notificationsFetchCmd's shape; More marks
// the reply for onResume's append-not-replace branch, matching
// ChatFetchedMsg's Resync.
func (m Model) fetchResumeMoreCmd(after string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		list, err := client.FetchResume(context.Background(), after, 0)
		return ResumeFetchedMsg{List: list, More: true, Err: err}
	}
}

// pickerNeedsFetch reports whether the picker should kick off a fetch
// for its next page: nothing already in flight, the list isn't
// exhausted, and the cursor rests on (or past) the last loaded row — the
// infinite-scroll trigger. Mirrors notificationsPanel.needsFetch
// (notifications.go), minus the first-open duty: Init already fires the
// picker's first page unconditionally, so this predicate is only ever
// consulted once rows are on screen — before that, pickerNext is also
// still "", so it would answer false regardless.
func (m Model) pickerNeedsFetch() bool {
	if m.pickerFetching || m.pickerNext == "" {
		return false
	}
	return m.cursor >= len(m.rows)-1
}

// maybeFetchMorePicker fires fetchResumeMoreCmd when pickerNeedsFetch
// says to, keeping the shared animation tick alive so the loader's phase
// moves — notificationsPanel.maybeFetchMore's twin.
func (m Model) maybeFetchMorePicker() (tea.Model, tea.Cmd) {
	if !m.pickerNeedsFetch() {
		return m, nil
	}
	m.pickerFetching = true
	return m, tea.Batch(m.fetchResumeMoreCmd(m.pickerNext), m.animate())
}

func (m Model) sendCmd(uuid, input string, width int, opts ...api.SendOpt) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		res, err := client.SendMessage(context.Background(), uuid, input, width, opts...)
		return SendResultMsg{Res: res, Err: err, Input: input}
	}
}

// ── Update ──────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.onResize(msg)
		// chrome.go/splash.go: onResize may have just armed the startup
		// splash (its own first-ready moment) — animate() is the seam that
		// actually starts the tick loop for it (a no-op whenever nothing
		// needs it, same as every other call site). The star sky's own
		// slow loop starts here too and never stops (ambient.go).
		return m, tea.Batch(m.animate(), m.startSky())
	case tea.MouseClickMsg:
		return m.onMouseClick(msg)
	case tea.MouseMotionMsg:
		return m.onMouseMotion(msg)
	case tea.MouseReleaseMsg:
		return m.onMouseRelease(msg)
	case tea.MouseWheelMsg:
		// Owner 2026-07-12: "make it also mouse wheel aware". Wheel
		// scrolls whatever surface is on screen — the conversation in
		// chat mode (three lines a notch, follow released/restored and
		// the scroll thumb woken exactly like the keyboard paths), the
		// cursor in the notifications overlay. animate() keeps ticks
		// flowing for the thumb fade, same as ctrl+u/d.
		mouse := msg.Mouse()
		switch {
		case m.mode == modeChat && mouse.Button == tea.MouseWheelUp:
			m.sc.ScrollUp(3)
			m.setFollow(m.sc.AtBottom())
			return m, m.animate()
		case m.mode == modeChat && mouse.Button == tea.MouseWheelDown:
			m.sc.ScrollDown(3)
			m.setFollow(m.sc.AtBottom())
			return m, m.animate()
		case m.mode == modeNotifications && mouse.Button == tea.MouseWheelUp:
			if m.notif.cursor > 0 {
				m.notif.cursor--
			}
			return m.maybeFetchMore()
		case m.mode == modeNotifications && mouse.Button == tea.MouseWheelDown:
			if m.notif.cursor < len(m.notif.rows)-1 {
				m.notif.cursor++
			}
			return m.maybeFetchMore()
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.onKey(msg)
	case ResumeFetchedMsg:
		return m.onResume(msg)
	case ConversationRenamedMsg:
		return m.onConversationRenamed(msg)
	case ConversationDeletedMsg:
		return m.onConversationDeleted(msg)
	case EntityPickerFetchedMsg:
		return m.onEntityPickerFetched(msg)
	case AiPickerFetchedMsg:
		return m.onAiPickerFetched(msg)
	case AiSettingsPatchedMsg:
		return m.onAiSettingsPatched(msg)
	case FootageProbedMsg:
		return m.onFootageProbed(msg)
	case VersionFetchedMsg:
		if msg.Err == nil && msg.Tag != "" {
			m.serverTag = msg.Tag
		}
		return m, nil
	case ChatFetchedMsg:
		return m.onChatFetched(msg)
	case SendResultMsg:
		return m.onSendResult(msg)
	case NotificationsFetchedMsg:
		return m.onNotificationsFetched(msg)
	case CableEventMsg:
		return m.onCableEvent(msg)
	case ConnStateMsg:
		return m.onConnState(msg)
	case resyncNowMsg:
		return m, msg.cmd
	case AnimTickMsg:
		return m.onAnimTick()
	case SkyTickMsg:
		return m.onSkyTick()
	case SuggestionsMsg:
		if msg.Seq == m.suggestSeq && m.input.Value() != "" {
			m.suggest = msg.S
			m.suggestSel = 0
			hint := ""
			if msg.S != nil {
				hint = msg.S.Ghost.NextHint
			}
			// micro.go effect 3: a hint that just changed starts its own
			// type-in from tick 0; an unchanged one is a no-op.
			m.beginGhostType(hint)
			// No refresh needed: the palette overlays the viewport now,
			// its open/close never moves the conversation.
			return m, m.animate()
		}
		return m, nil
	case spinner.TickMsg:
		if len(m.pending) == 0 {
			return m, nil // pending drained: let the tick loop die
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		m.refreshViewport()
		return m, cmd
	}
	return m, nil
}

func (m Model) onResize(msg tea.WindowSizeMsg) Model {
	m.width, m.height = msg.Width, msg.Height
	if m.mode == modeFootage && m.footage.step == footageStepFolder {
		// FolderPicker sizes itself off its OWN width/height fields (its
		// View doc comment) — it needs the same WindowSizeMsg Model itself
		// just consumed, not a derived value.
		fp, _ := m.footage.folder.Update(msg)
		m.footage.folder = fp
	}
	opts := []render.Option{}
	if m.plainRender {
		opts = append(opts, render.WithPlain())
	}
	if m.glamourStyle != "" {
		opts = append(opts, render.WithStyle(m.glamourStyle))
	}
	opts = append(opts, render.WithTruecolor(m.truecolor), render.WithNow(m.now))
	m.renderer = render.New(m.contentWidth(), opts...)
	renderer := m.renderer
	// chrome.go effect 4: wrapEventLinks OSC-8-wraps a share-reply's URL —
	// a thin post-process over renderer.Event(ev)'s own output, never a
	// change to render.go itself.
	m.transcript.SetRenderer(func(ev api.Event, _ int) string { return wrapEventLinks(ev, renderer.Event(ev)) })

	firstReady := !m.ready
	if !m.ready {
		m.sc = chatScroller{width: m.width, height: m.chatViewportHeight()}
	} else {
		m.sc.SetWidth(m.width)
		m.sc.SetHeight(m.chatViewportHeight())
	}
	// Owner smoke feedback 2026-07-12: the bottom chrome follows the
	// content column — a capped conversation over a full-width prompt
	// read as two different apps. The whole UI is ONE column now.
	m.input.SetWidth(m.contentWidth() - len(m.input.Prompt) - 1)
	m.ready = true
	if firstReady {
		// splash.go effect 1: the terminal just became ready for the very
		// first time — arm the startup splash (never on a later resize).
		m = m.maybeStartSplash()
	}
	m.refreshViewport()
	return m
}

func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.tourActive() {
		switch msg.String() {
		case "esc", "ctrl+c":
			// tour.go's own escape hatch: a live demo must never be one
			// slipped keystroke from vanishing — esc/ctrl+c abort the
			// SCRIPT, not the whole program, dropping straight into
			// normal interactive use (ctrl+c's usual tea.Quit is
			// deliberately shadowed for the duration of the tour only).
			// The tour's OWN synthetic esc — the /notifications step's
			// timed close, stepTourDwelling — never reaches here: it
			// calls onNotificationsKey directly rather than routing back
			// through onKey, so it can never trip this branch itself.
			m.tour.active = false
			m.tour.caption = ""
			return m, nil
		}
	}
	if msg.String() == "ctrl+c" {
		// Crush-style confirm (owner 2026-07-12): the first ctrl+c ARMS
		// the quit and says so on the status row; a second within the
		// window actually quits. Any other key disarms below.
		if m.quitArmed > 0 {
			return m, tea.Quit
		}
		m.quitArmed = quitArmTicks
		return m, m.animate()
	}
	if m.quitArmed > 0 {
		m.quitArmed = 0 // any other key stands the quit down
	}
	if m.splashActive() {
		// splash.go effect 1: ANY key skips the startup splash instantly —
		// no rise-away animation plays, this same keystroke never reaches
		// the picker/chat dispatch below.
		m.skipSplash()
		return m, nil
	}
	switch m.mode {
	case modePicker:
		return m.onPickerKey(msg)
	case modeNotifications:
		return m.onNotificationsKey(msg)
	case modeCommandPalette:
		return m.onCtrlKKey(msg)
	case modeEntityPicker:
		return m.onEntityPickerKey(msg)
	case modeAiPicker:
		return m.onAiPickerKey(msg)
	case modeFootage:
		return m.onFootageKey(msg)
	case modeWarn:
		return m.onWarnKey(msg)
	}
	return m.onChatKey(msg)
}

func (m Model) onPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// A live rename input owns every key until Enter/Esc — the web's own
	// contract (pito--rename's input listener runs before the
	// document-level #onKey, so arrow/d/n never reach row navigation while
	// the field is focused).
	if m.pickerRenaming != "" {
		return m.onPickerRenameKey(msg)
	}
	switch msg.String() {
	case "j", "down":
		m.pickerDeleteArmed = 0 // moving the highlight disarms dd (web parity)
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
		return m.maybeFetchMorePicker()
	case "k", "up":
		m.pickerDeleteArmed = 0
		if m.cursor > 0 {
			m.cursor--
		}
		return m.maybeFetchMorePicker()
	case "enter":
		if len(m.rows) == 0 {
			return m, nil
		}
		row := m.rows[m.cursor]
		// The picker slides back down before the mode actually switches
		// to chat (spring.go's stepOverlays) — closeAction carries
		// whatever onKey would otherwise have done immediately.
		if row.isNew {
			// No uuid yet — the first send creates the conversation.
			m.mode = modeChat
			return m, nil
		}
		m.mode = modeChat
		m.conv.UUID = row.uuid
		m.conv.DisplayName = row.title
		return m, m.fetchChatCmd(row.uuid, false)
	case "esc":
		if m.pickerDeleteArmed > 0 {
			// Escape disarms dd without closing the picker (web parity:
			// resume_controller.js's #onKey "if (this.armedRow) disarm").
			m.pickerDeleteArmed = 0
			return m, nil
		}
		// Only a picker OVER an open conversation can close back to it
		// (the /resume path); the startup picker has nowhere to go.
		if m.conv.UUID == "" {
			return m, nil
		}
		m.mode = modeChat
		return m, nil
	case "n":
		return m.openPickerRename()
	case "d":
		return m.onPickerDeleteKey()
	}
	return m, nil
}

// openPickerRename starts inline-rename on the highlighted row — the
// picker's `n` key, mirroring the web's pito--rename #startRename (double
// click there, `n` here since there's no mouse). Ignored for the "new
// conversation" sentinel row, which has no uuid to rename.
func (m Model) openPickerRename() (tea.Model, tea.Cmd) {
	if len(m.rows) == 0 || m.cursor >= len(m.rows) {
		return m, nil
	}
	row := m.rows[m.cursor]
	if row.isNew {
		return m, nil
	}
	m.pickerDeleteArmed = 0

	ti := textinput.New()
	ti.Prompt = ""
	ti.SetValue(row.title)
	ti.CursorEnd()
	styles := ti.Styles()
	styles.Cursor.Color = render.ColorAccent
	ti.SetStyles(styles)
	if w := m.contentWidth() - 4; w > 4 {
		ti.SetWidth(w)
	}
	ti.Focus()

	m.pickerRenaming = row.uuid
	m.pickerRenameInput = ti
	return m, textinput.Blink
}

// onPickerRenameKey drives the inline rename input: Enter submits (a
// blank/whitespace value cancels without a network call, matching the
// web's #commitRename), Esc cancels, everything else edits the field.
func (m Model) onPickerRenameKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		uuid := m.pickerRenaming
		newTitle := strings.TrimSpace(m.pickerRenameInput.Value())
		m.pickerRenaming = ""
		if newTitle == "" {
			return m, nil
		}
		// Optimistic update — the web restyles the row before the PATCH
		// lands too (pito--rename #commitRename).
		for i := range m.rows {
			if m.rows[i].uuid == uuid {
				m.rows[i].title = newTitle
			}
		}
		if m.conv.UUID == uuid {
			m.conv.DisplayName = newTitle
		}
		return m, m.renameConversationCmd(uuid, newTitle)
	case "esc":
		m.pickerRenaming = ""
		return m, nil
	default:
		var cmd tea.Cmd
		m.pickerRenameInput, cmd = m.pickerRenameInput.Update(msg)
		return m, cmd
	}
}

// renameConversationCmd PATCHes the rename (api.Client.RenameConversation
// — the same /chat/:uuid endpoint the web's inline rename hits).
func (m Model) renameConversationCmd(uuid, title string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		newTitle, err := client.RenameConversation(context.Background(), uuid, title)
		return ConversationRenamedMsg{UUID: uuid, Title: newTitle, Err: err}
	}
}

// onConversationRenamed absorbs the rename PATCH's reply. A failure is
// left as a silent no-op past the auth check — like the web's
// #commitRename ("leave the optimistic text in place... simple: just
// keep what's shown") there is no earlier title cached locally to roll
// back to.
func (m Model) onConversationRenamed(msg ConversationRenamedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		if isUnauthorized(msg.Err) {
			m.needsLogin = true
		}
		return m, nil
	}
	for i := range m.rows {
		if m.rows[i].uuid == msg.UUID {
			m.rows[i].title = msg.Title
		}
	}
	if m.conv.UUID == msg.UUID {
		m.conv.DisplayName = msg.Title
	}
	return m, nil
}

// onPickerDeleteKey handles the picker's `d` key — the web's vim-style dd
// chord (resume_controller.js): a first `d` arms the highlighted row
// (pickerDeleteArmTicks ~500ms, ticked down in onAnimTick, mirroring the
// JS setTimeout), a second `d` within that window deletes it. The "new
// conversation" sentinel row has nothing to delete.
func (m Model) onPickerDeleteKey() (tea.Model, tea.Cmd) {
	if len(m.rows) == 0 || m.cursor >= len(m.rows) {
		return m, nil
	}
	row := m.rows[m.cursor]
	if row.isNew {
		return m, nil
	}
	if m.pickerDeleteArmed > 0 {
		m.pickerDeleteArmed = 0
		return m, m.deleteConversationCmd(row.uuid)
	}
	m.pickerDeleteArmed = pickerDeleteArmTicks
	return m, m.animate()
}

// deleteConversationCmd DELETEs the conversation (api.Client.
// DeleteConversation — the same /chat/:uuid endpoint the web's dd chord
// hits).
func (m Model) deleteConversationCmd(uuid string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		err := client.DeleteConversation(context.Background(), uuid)
		return ConversationDeletedMsg{UUID: uuid, Err: err}
	}
}

// onConversationDeleted absorbs the delete DELETE's reply: on success the
// row is dropped from the picker and, if it was the conversation the
// picker was opened OVER, m.conv is cleared — the web's post-delete rule
// (resume_controller.js #deleteConversation: "Leave the conversation if
// it's the one open" navigates home; the tui's "home" is the picker
// itself, already on screen, so clearing conv is the equivalent — it
// drops the picker's Esc-back-to-chat affordance for a conversation that
// no longer exists).
func (m Model) onConversationDeleted(msg ConversationDeletedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		if isUnauthorized(msg.Err) {
			m.needsLogin = true
		}
		m.pickerNotice = "could not delete that conversation"
		return m, nil
	}
	for i, row := range m.rows {
		if row.uuid == msg.UUID {
			m.rows = append(m.rows[:i], m.rows[i+1:]...)
			if m.cursor >= len(m.rows) && m.cursor > 0 {
				m.cursor--
			}
			break
		}
	}
	if m.conv.UUID == msg.UUID {
		m.conv = api.Conversation{}
	}
	return m, nil
}

func (m Model) onChatKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Palette interactions first — they own a few keys while open.
	if m.suggest != nil && len(m.suggest.MenuItems) > 0 {
		switch msg.String() {
		case "down", "ctrl+n":
			m.suggestSel = (m.suggestSel + 1) % len(m.suggest.MenuItems)
			return m, nil
		case "up", "ctrl+p":
			m.suggestSel = (m.suggestSel - 1 + len(m.suggest.MenuItems)) % len(m.suggest.MenuItems)
			return m, nil
		case "tab":
			m.acceptSuggestion()
			m.refreshViewport()
			// The accepted insert can be a Shift+R "#… @ai" handle —
			// the ">" pulse starts here too.
			return m, tea.Batch(m.suggestCmd(), m.animate())
		case "space":
			// Web parity (v1.6.0 suggestions_controller.js): Space
			// dismisses the palette WITHOUT swallowing the keystroke —
			// the space still types into the input below.
			m.suggest = nil
			m.refreshViewport()
			// fall through to the input by NOT returning here
		case "esc":
			m.suggest = nil
			m.refreshViewport()
			return m, nil
		}
	}

	switch msg.String() {
	case "ctrl+k":
		// The web's global command palette (command_palette_controller.js)
		// — chat mode only here: the picker is its own full-screen list.
		return m.openCtrlK()
	case "ctrl+f":
		// The ctrl+f "update footage" flow (footage.go) — mirrors the
		// action hint's own auth gate (actionHints hides the hint entirely
		// for an unauthenticated session; here the key itself is a no-op
		// rather than opening a flow that ends in a 401).
		if m.needsLogin {
			return m, nil
		}
		return m.openFootageGate()
	case "shift+tab":
		// Channel cycler — only while its hint is live (web parity).
		if hintMode(m.input.Value()) == "channel" && len(m.channels) > 0 {
			m.scopeChannel = cycleNext(m.channels, m.scopeChannel)
			return m, m.patchScopeCmd()
		}
		return m, nil
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		m.recordHistory(text)
		if strings.EqualFold(text, "/resume") {
			// Client-side, like /notifications: the server's own /resume
			// answers "That one needs a browser" to non-browser clients —
			// the web opens its sidebar, the TUI opens ITS conversation
			// picker (owner bug 2026-07-12: "/resume doesn't work
			// anymore"). Fresh page-1 fetch, fresh slide-in.
			m.input.Reset()
			m.suggest = nil
			m.mode = modePicker
			m.rows = nil
			m.cursor = 0
			m.pickerNext = ""
			m.pickerNotice = ""
			m.pickerFetching = true
			return m, tea.Batch(m.fetchResumeCmd(), m.animate())
		}
		if noun, command, ok := entityPickerTrigger(text); ok {
			// Bare `show game` / `show vid` opens the web's picker
			// sidebar — the TUI opens ITS picker (entitypicker.go); the
			// id-carrying forms still round-trip to the server.
			m.input.Reset()
			m.suggest = nil
			return m.openEntityPicker(noun, command)
		}
		if aiPickerTrigger(text) {
			// Bare `/config ai` opens the web's model-picker overlay —
			// the TUI opens ITS picker (aipicker.go, tui-needs ask #3);
			// every other /config form still round-trips untouched.
			m.input.Reset()
			m.suggest = nil
			return m.openAiPicker()
		}
		if strings.EqualFold(text, "/notifications") {
			// Client-side grammar, like /login: intercepted before it ever
			// reaches sendCmd. The bare `notifications` tool (no slash, or
			// with trailing args) is NOT this — it still round-trips to the
			// server below, untouched.
			m.input.Reset()
			m.suggest = nil
			return m.openNotifications()
		}
		// The web's #syncHidden: scope params ride ONLY when their hint
		// is live at send time; the server falls back to the persisted
		// conversation scope otherwise.
		var opts []api.SendOpt
		switch hintMode(text) {
		case "channel":
			if len(m.channels) > 0 {
				opts = append(opts, api.WithChannelScope(m.scopeChannel))
			}
		case "period":
			opts = append(opts, api.WithPeriodScope(m.scopePeriod))
		}
		m.input.Reset()
		m.suggest = nil
		m.sounds.Send()
		// viewport_width is PIXELS on the wire (the web sends element
		// widths); approximate the terminal at ~8px per cell so wide
		// terminals get the same column auto-fill the web enjoys.
		return m, m.sendCmd(m.conv.UUID, text, m.contentWidth()*8, opts...)
	case "ctrl+d", "pgdown":
		// PgDn rides along ctrl+d — the tui way (owner 2026-07-12:
		// optional but blessed; shift+↑/↓ stay the primary scroll).
		m.sc.HalfPageDown()
		m.setFollow(m.sc.AtBottom())
		// setFollow may have just started the scroll-thumb's fade-out
		// (micro.go effect 4) — animate() is the seam that actually keeps
		// ticks flowing for it; a no-op whenever nothing needs them.
		return m, m.animate()
	case "ctrl+u", "pgup":
		m.sc.HalfPageUp()
		m.setFollow(m.sc.AtBottom())
		return m, m.animate()
	case "up":
		// Input-history recall (history_controller.js parity). The
		// palette-open guard already ran above — while it shows, ↑/↓
		// belong to it, exactly like the web's suggestions guard.
		if len(m.histEntries) == 0 {
			return m, nil
		}
		if m.histPrefix == nil {
			// First ↑ starts the recall session: snapshot the buffer as
			// the search prefix (empty buffer matches everything).
			prefix := m.input.Value()
			m.histPrefix = &prefix
			m.histDraft = prefix
			m.histMatches = nil
			for _, e := range m.histEntries {
				if strings.HasPrefix(e, prefix) {
					m.histMatches = append(m.histMatches, e)
				}
			}
		}
		if m.histIndex+1 >= len(m.histMatches) {
			return m, nil // no match / already at the oldest — no wrap
		}
		m.histIndex++
		m.applyHistoryEntry(m.histMatches[m.histIndex])
		return m, m.animate() // a recalled "@ai …" entry starts the ">" pulse
	case "down":
		if m.histPrefix == nil || m.histIndex == -1 {
			return m, nil // no session / already at the snapshot draft
		}
		m.histIndex--
		if m.histIndex < 0 {
			// Back to the draft; the prefix survives so a further ↑
			// resumes the same walk (web behavior).
			m.histIndex = -1
			m.applyHistoryEntry(m.histDraft)
			return m, m.animate() // the restored draft may itself be @ai
		}
		m.applyHistoryEntry(m.histMatches[m.histIndex])
		return m, m.animate() // a recalled "@ai …" entry starts the ">" pulse
	case "shift+up":
		// Web parity: shift+↑/↓ scroll the conversation — and unlike the
		// vim keys they work mid-typing (they never collide with input).
		m.sc.ScrollUp(1)
		m.setFollow(m.sc.AtBottom())
		return m, m.animate()
	case "shift+down":
		m.sc.ScrollDown(1)
		m.setFollow(m.sc.AtBottom())
		return m, m.animate()
	case "ctrl+home":
		// Web parity (pito--scroll-nav#jumpTop): jump to the very start.
		m.sc.GotoTop()
		m.setFollow(m.sc.AtBottom())
		return m, m.animate()
	case "ctrl+end":
		// Web parity (#jumpBottom smooth-scrolls): re-engage follow and
		// let the house glide carry the viewport down (easeTowardBottom;
		// more than two screens away still lands instantly, its rule).
		m.setFollow(true)
		m.scrollEasing = true
		return m, m.animate()
	}

	if msg.String() == "ctrl+space" {
		// Ctrl+Space stands in for the web's shift+space (terminals
		// cannot report shift+space): period cycler while analyzing.
		if hintMode(m.input.Value()) == "period" {
			m.scopePeriod = cycleNext(periods, m.scopePeriod)
			return m, m.patchScopeCmd()
		}
		return m, nil
	}

	if m.input.Value() == "" {
		switch msg.String() {
		case "?":
			m.showHelp = !m.showHelp
			// chrome.go effect 2: animate() starts the footer's own
			// open/close spring ticking (a no-op off-truecolor or with the
			// flag off — footerAnimating() answers false either way).
			return m, m.animate()
		case "R":
			// Shift+R at an empty prompt (web parity, caret-0 rule):
			// prefill the newest live reply handle; several → the palette
			// becomes the picker; none → the R types through below.
			handles := m.transcript.LiveHandles()
			switch {
			case len(handles) == 1:
				m.input.SetValue(replyPrefill(handles[0]))
				m.input.CursorEnd()
				m.suggest = nil
				// An AI handle prefills "#… @ai " — the ">" starts
				// pulsing, so the loop must start with it.
				return m, tea.Batch(m.suggestCmd(), m.animate())
			case len(handles) > 1:
				items := make([]api.Suggestion, 0, len(handles))
				for _, h := range handles {
					insert := strings.TrimRight(replyPrefill(h), " ")
					items = append(items, api.Suggestion{Label: insert, Insert: insert})
				}
				m.suggest = &api.Suggestions{MenuItems: items}
				m.suggestSel = 0
				m.beginGhostType("") // this menu carries no ghost hint of its own
				m.refreshViewport()
				return m, nil
			}
		}
		// The v1 vim scroll letters (j/k/g/G at an empty prompt) are GONE
		// — 2.0.0's grammar starts real commands with them ("glance",
		// "jobs"), so the first letter of a command typed at rest was
		// eaten as a scroll (live-observed 2026-07-12: the server
		// received "lance game 3"). Scrolling keeps ctrl+u/d, pgup/pgdn,
		// shift+↑/↓ and the mouse wheel; every letter belongs to the
		// input.
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		// A real edit ends the recall session — the next ↑ re-snapshots
		// from the now-current buffer (history_controller.js #onInput).
		m.resetHistoryRecall()
		if m.input.Value() == "" {
			m.suggest = nil
			m.refreshViewport()
			return m, cmd
		}
		// m.animate kicks the tick loop the moment the buffer enters the
		// @ai mood (aiPromptLive) — a no-op whenever the loop already
		// runs or nothing needs it.
		return m, tea.Batch(cmd, m.suggestCmd(), m.animate())
	}
	return m, cmd
}

// applyHistoryEntry puts a recalled entry (or the restored draft) into
// the prompt, caret at the end. Deliberately NO suggestCmd here: the
// web flags this synthetic input historyRecall so the palette does not
// reopen over a recalled slash entry and steal the next ↑/↓.
func (m *Model) applyHistoryEntry(text string) {
	m.input.SetValue(text)
	m.input.CursorEnd()
	m.suggest = nil
	m.refreshViewport()
}

// recordHistory prepends a sent input: consecutive duplicates collapse,
// the list caps at 50, and any recall session ends so the next ↑ starts
// a fresh search from the cleared buffer (history_controller.js #onSubmit).
func (m *Model) recordHistory(text string) {
	if len(m.histEntries) == 0 || m.histEntries[0] != text {
		m.histEntries = append([]string{text}, m.histEntries...)
		if len(m.histEntries) > 50 {
			m.histEntries = m.histEntries[:50]
		}
	}
	m.histDraft = ""
	m.resetHistoryRecall()
}

// resetHistoryRecall ends any active recall session (keeps the entries).
func (m *Model) resetHistoryRecall() {
	m.histIndex = -1
	m.histPrefix = nil
	m.histMatches = nil
}

// seedHistory rebuilds the entry list from the transcript's echo events —
// the web's page-load seeding (sent_history in conversations/show.html.erb).
// Skipped mid-recall so a resync can't yank the list out from under a walk;
// entries typed this session are already in the transcript as echoes.
func (m *Model) seedHistory() {
	if m.histPrefix != nil {
		return
	}
	m.histEntries = m.transcript.EchoTexts(50)
}

// VersionFetchedMsg carries GET /version's tag (or its failure — a
// failed fetch leaves the dot standing alone; the next reconnect
// retries).
type VersionFetchedMsg struct {
	Tag string
	Err error
}

// fetchVersionCmd asks the server for its build tag — only ever fired
// for non-dev hosts (resolvedServerTag answers "dev" locally for those).
func (m Model) fetchVersionCmd() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		tag, err := client.FetchVersion(context.Background())
		return VersionFetchedMsg{Tag: tag, Err: err}
	}
}

// eventNeedsTicks reports whether ONE event keeps its turn on the 40ms
// animation loop. The distinction that matters is pending vs resolved:
// a RESOLVED thinking line or confirmation renders static, and marking
// them anyway put every backfilled turn on a permanent 25fps re-render
// treadmill (owner smoke 2026-07-12: "everything seems very very slow")
// — the transcript is full of resolved thinking events, one per turn.
func eventNeedsTicks(ev api.Event) bool {
	switch ev.Kind {
	case api.KindAi:
		return true // the AI chrome's gradient bar rides the phase while pending AND done
	case api.KindThinking, api.KindConfirmation, api.KindConfirmationFollowUp:
		var p struct {
			Resolved bool `json:"resolved"`
		}
		_ = json.Unmarshal(ev.Payload, &p)
		return !p.Resolved
	}
	return render.HasShimmer(ev.Payload)
}

// reconcileTurnShimmer re-evaluates one turn's animation membership after
// a replace (a thinking resolve, a confirmation outcome): when none of
// the turn's events needs ticks anymore, the turn leaves the shimmer map
// and — once nothing else is alive — the 40ms loop goes quiet again.
func (m *Model) reconcileTurnShimmer(turnID int64) {
	if !m.shimmer[turnID] {
		return
	}
	for _, ev := range m.transcript.TurnEvents(turnID) {
		if eventNeedsTicks(ev) {
			return
		}
	}
	delete(m.shimmer, turnID)
}

// resyncNowMsg fires a deferred re-sync (mutation safety net).
type resyncNowMsg struct{ cmd tea.Cmd }

// echoLocally appends a synthetic echo turn for inputs the server
// accepts without creating a turn (reply mutations). Negative IDs keep
// synthetic events out of the server-ID space, so merges ignore them.
func (m *Model) echoLocally(input string) {
	if input == "" {
		return
	}
	m.syntheticID--
	m.transcript.Append(api.Event{
		ID:        m.syntheticID,
		TurnID:    m.syntheticID,
		Kind:      api.KindEcho,
		Payload:   []byte(`{"text":` + strconv.Quote(input) + `}`),
		CreatedAt: time.Now(),
	})
}

// suggestCmd asks the server-side palette about the current input. The
// seq counter lets stale replies lose to fresh keystrokes.
func (m *Model) suggestCmd() tea.Cmd {
	m.suggestSeq++
	seq := m.suggestSeq
	client := m.client
	uuid := m.conv.UUID
	input := m.input.Value()
	cursor := m.input.Position()
	return func() tea.Msg {
		s, err := client.Suggest(context.Background(), uuid, input, cursor)
		if err != nil {
			return SuggestionsMsg{Seq: seq, S: nil}
		}
		return SuggestionsMsg{Seq: seq, S: s}
	}
}

// acceptSuggestion replaces the trailing token with the selection.
func (m *Model) acceptSuggestion() {
	item := m.suggest.MenuItems[m.suggestSel]
	insert := item.Insert
	if insert == "" {
		insert = item.Label
	}
	value := m.input.Value()
	cut := strings.LastIndexByte(strings.TrimRight(value, " "), ' ') + 1
	if cut < 0 {
		cut = 0
	}
	next := value[:cut] + insert
	if !strings.HasSuffix(next, " ") {
		next += " "
	}
	m.input.SetValue(next)
	m.input.CursorEnd()
	m.suggest = nil
}

// onResume absorbs one /resume.json page. The first page (More false)
// replaces m.rows wholesale — the exact byte-identical path a cursorless
// server always takes, since its single reply is indistinguishable from
// an exhausted page 1 (see ResumeList's doc comment). A pagination
// follow-on (More true, tui-needs ask 9a) appends its rows below what's
// already on screen instead, then chains into maybeFetchMorePicker so a
// short page — or a cursor already resting on the new last row when it
// lands — keeps pulling, mirroring notificationsPanel.onNotificationsFetched.
func (m Model) onResume(msg ResumeFetchedMsg) (tea.Model, tea.Cmd) {
	m.pickerFetching = false
	if msg.Err != nil {
		// pickerNext is left untouched on error: a later scroll nudge
		// retries the SAME cursor via pickerNeedsFetch — no separate
		// retry path to maintain (mirrors notificationsPanel's own error
		// handling).
		m.loadErr = "could not load conversations: " + msg.Err.Error()
		return m, nil
	}
	m.loadErr = ""
	if msg.More {
		m.rows = append(m.rows, resumeListRows(msg.List)...)
	} else {
		m.rows = pickerRows(msg.List)
		m.needsLogin = false // resume is auth-gated too
	}
	if msg.List.Notifications != nil {
		// The ✉ seed for the fresh-chat boot (and a free refresh any
		// other time a resume page lands).
		m.unread = msg.List.Notifications.Unread
	}
	m.pickerNext = msg.List.NextCursor
	return m.maybeFetchMorePicker()
}

func (m Model) onChatFetched(msg ChatFetchedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		switch {
		case isUnauthorized(msg.Err):
			m.needsLogin = true
		case errors.Is(msg.Err, api.ErrNotFound):
			// Unknown uuid (stale config, deleted conversation): back to
			// the picker rather than a dead chat.
			m.mode = modePicker
			m.conv = api.Conversation{}
			m.cursor = 0
			m.pickerNotice = "that conversation does not exist anymore"
			return m, tea.Batch(m.fetchResumeCmd(), m.animate())
		default:
			m.pushNotice("scrollback fetch failed: " + msg.Err.Error())
		}
		m.refreshViewport()
		return m, nil
	}
	m.conv = msg.Page.Conversation
	if msg.Page.Conversation.Context != nil {
		m.meterCtx = msg.Page.Conversation.Context
	}
	if msg.Page.Notifications != nil {
		m.unread = msg.Page.Notifications.Unread
	}
	if msg.Page.Conversation.Scope != nil {
		if msg.Page.Conversation.Scope.Channel != "" {
			m.scopeChannel = msg.Page.Conversation.Scope.Channel
		}
		if msg.Page.Conversation.Scope.Period != "" {
			m.scopePeriod = msg.Page.Conversation.Scope.Period
		}
	}
	if len(msg.Page.Channels) > 0 {
		m.channels = msg.Page.Channels
	}
	m.needsLogin = false // the backfill is auth-gated: fetching it proves the session
	var tagCmd tea.Cmd
	if m.client != nil && !isDevHost(m.client.BaseURL().Hostname()) {
		// Non-dev host: (re)fetch the server tag — the initial paint and
		// every reconnect re-sync land here, mirroring the web's
		// cable-health version check cadence.
		tagCmd = m.fetchVersionCmd()
	}
	m.transcript.Merge(msg.Page.Events)
	m.seedHistory()
	for _, ev := range msg.Page.Events {
		if m.truecolor && eventNeedsTicks(ev) {
			m.shimmer[ev.TurnID] = true
		}
	}
	// A fetched page can carry the first events of turns still marked
	// pending (created-conversation paint, reconnect re-sync) — the cable
	// isn't the only way a turn shows up.
	cleared := false
	for turnID := range m.pending {
		if m.transcript.HasTurn(turnID) {
			delete(m.pending, turnID)
			cleared = true
		}
	}
	if cleared {
		m.sounds.Receive()
	}
	m.refreshViewport()
	if !m.cableStarted && m.conv.UUID != "" && m.connect != nil {
		m.connect(m.conv.UUID)
		m.cableStarted = true
	}
	return m, tea.Batch(m.animate(), tagCmd)
}

func (m Model) onSendResult(msg SendResultMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		if isUnauthorized(msg.Err) {
			m.needsLogin = true
		} else {
			m.pushNotice("send failed: " + msg.Err.Error())
		}
		m.refreshViewport()
		return m, nil
	}
	res := msg.Res
	switch {
	case res.Notice != nil:
		// The server said no in its own voice — show it verbatim.
		m.pushNotice(res.Notice.Text())
		m.refreshViewport()
		return m, nil
	case res.CreatedUUID != "":
		// First send of a fresh conversation: adopt the uuid, mark its
		// turn pending, then paint and subscribe exactly like a picked
		// conversation.
		m.conv.UUID = res.CreatedUUID
		if res.TurnID != 0 && !m.transcript.HasTurn(res.TurnID) {
			m.pending[res.TurnID] = true
		}
		return m, tea.Batch(m.fetchChatCmd(res.CreatedUUID, false), m.spin.Tick)
	default:
		if res.TurnID == 0 {
			// turn_id is null for reply MUTATIONS (sort/with/without…):
			// FollowUpDispatchJob edits the event ASYNC and mirrors a
			// cable replace. Echo locally (the server won't), and add a
			// delayed re-sync as the cable's safety net — immediate
			// fetching would race the job and merge stale bytes.
			m.echoLocally(msg.Input)
			m.refreshViewport()
			uuid := m.conv.UUID
			resync := m.fetchChatCmd(uuid, true)
			return m, tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg {
				return resyncNowMsg{cmd: resync}
			})
		}
		if m.transcript.HasTurn(res.TurnID) {
			// The cable beat the HTTP ack — the turn's events already
			// rendered. Marking it pending now would strand the spinner.
			return m, nil
		}
		m.pending[res.TurnID] = true
		m.refreshViewport()
		return m, m.spin.Tick
	}
}

func (m Model) onCableEvent(msg CableEventMsg) (tea.Model, tea.Cmd) {
	// Every delivered message IS cable activity — the presence dot takes
	// its one green breath regardless of message type (ambient.go); the
	// follow-glide it may start below needs the same ticks.
	m.beginDotPulse()
	ev := msg.M.Event
	switch msg.M.Type {
	case cable.TypeEventAppend:
		if m.pending[ev.TurnID] {
			delete(m.pending, ev.TurnID)
			m.sounds.Receive()
		}
		m.transcript.Append(ev)
		if m.truecolor && eventNeedsTicks(ev) {
			// Pending thinking rides the shimmer map too: its braille
			// frame and network shimmer live off the same Touch loop.
			m.shimmer[ev.TurnID] = true
		}
		if ev.Kind == api.KindError {
			// micro.go effect 1: the error's own arrival on the transcript
			// IS the shake's trigger — pushed immediately (not deferred to
			// the next tick) so this same refreshViewport() below already
			// paints tick 0's offset.
			m.beginErrorShake(ev)
			m.pushShakeOffsets()
		}
	case cable.TypeEventReplace:
		// A replace proves the turn is flowing too — the thinking
		// resolve (event.replace, resolved: true) is THE turn-done
		// signal per the contract, so pending must not survive it.
		delete(m.pending, ev.TurnID)
		m.transcript.Replace(ev)
		m.reconcileTurnShimmer(ev.TurnID)
		// Fills swap at full height — the follow-glide absorbs growth
		// (reveal springs purged 2026-07-12; the old re-reveal shrank
		// content for a frame and yanked a following viewport backward).
	case cable.TypeEventAiBlock:
		m.applyAiBlock(msg.M.EventID, msg.M.Index, msg.M.Block)
	case cable.TypeEventAiStatus:
		m.applyAiStatus(msg.M.EventID, msg.M.Text)
	case cable.TypeConversationUpdate:
		// ripple.go effect 1: the message's own arrival on the cable IS
		// the status-bar ripple's trigger, regardless of which field(s)
		// below actually changed this time.
		m.beginRipple()
		if msg.M.Context != nil {
			m.meterCtx = msg.M.Context
		}
		if msg.M.Notifications != nil {
			if msg.M.Notifications.Unread != m.unread {
				// ripple.go effect 2: only an ACTUAL change rolls the ✉
				// odometer; an unchanged count is a silent no-op.
				m.beginUnreadRoll(msg.M.Notifications.Unread)
			}
		}
	default:
		return m, nil // unknown stream message type: ignore
	}
	m.refreshViewport()
	return m, m.animate()
}

// applyAiBlock merges one streamed block (event.ai_block) into eventID's
// ai payload at Index. The server streams blocks into a pending ai event
// ahead of the authoritative event.replace, and indices can arrive out of
// order, so the slot is grown with empty placeholder blocks as needed.
// DecodeAiPayload hands back each existing block's own untouched raw
// bytes, so only the target index and the "blocks" array itself change —
// every other block, and every unknown top-level payload key, rides
// through byte-identical rather than round-tripping through the typed
// AiPayload as a whole (which would silently drop fields the TUI doesn't
// know about yet). An eventID with no event yet on the transcript (a
// block racing ahead of its event.append) is a silent no-op — the
// eventual event.replace reconciles it either way.
func (m *Model) applyAiBlock(eventID int64, index int, block json.RawMessage) {
	if index < 0 {
		return
	}
	ev, ok := m.transcript.MutateEventPayload(eventID, func(raw json.RawMessage) (json.RawMessage, bool) {
		p, err := api.DecodeAiPayload(raw)
		if err != nil {
			return nil, false
		}
		for len(p.Blocks) <= index {
			p.Blocks = append(p.Blocks, api.AiBlock{Raw: json.RawMessage(`{}`)})
		}
		p.Blocks[index] = api.AiBlock{Raw: append(json.RawMessage(nil), block...)}

		var fields map[string]any
		if json.Unmarshal(raw, &fields) != nil {
			return nil, false
		}
		blocks := make([]json.RawMessage, len(p.Blocks))
		for i, b := range p.Blocks {
			blocks[i] = b.Raw
		}
		fields["blocks"] = blocks
		out, err := json.Marshal(fields)
		if err != nil {
			return nil, false
		}
		return out, true
	})
	if !ok {
		return
	}
	_ = ev // reveal springs purged (owner 2026-07-12) — the mutate alone repaints
}

// applyAiStatus sets/replaces the client-side "status_line" payload key
// (event.ai_status) — a convention this client invents to carry the
// server's ephemeral status prose ("Scouring the internet…") through the
// same JSON payload every other renderer reads; render/ai.go's pending
// branch shows it in place of the bare ellipsis. Same unknown-field
// preservation as applyAiBlock, and the same silent no-op for a block
// racing ahead of its event.append. A later event.replace clears the key
// naturally: Replace installs the server's own event wholesale, and the
// server payload never carries "status_line" itself.
func (m *Model) applyAiStatus(eventID int64, text string) {
	m.transcript.MutateEventPayload(eventID, func(raw json.RawMessage) (json.RawMessage, bool) {
		var fields map[string]any
		if json.Unmarshal(raw, &fields) != nil {
			return nil, false
		}
		fields["status_line"] = text
		out, err := json.Marshal(fields)
		if err != nil {
			return nil, false
		}
		return out, true
	})
}

// Shimmer runs indefinitely, like the web (owner call, 2026-07-05) — the
// tick loop lives as long as shimmer-marked turns are in the transcript.
// 40ms ticks (25fps — owner call 2026-07-06: 12.5fps read as visible
// ticking) with a ~2.7s cycle; per-element stagger lives in the
// renderer (phaseOffset), so the shared phase never syncs neighbors —
// EXCEPT during the shimmer conductor's own ~2s sweep window every ~30s
// of alive time, where SetConductor deliberately collapses every
// element's offset onto this same shared phase (ambient.go).
const (
	// 16ms ≈ 60fps (owner 2026-07-12, second pass: 30fps still read as
	// "something keeping the animation in place" on a 280Hz OLED — the
	// whole loop runs 60 now; the dirty-gated refresh keeps chrome
	// frames cheap and v2's cell-diff renderer only writes what moved).
	// shimmerStep rescales to hold the sweep's ~2.7s period.
	shimmerTick = 16 * time.Millisecond
	shimmerStep = 0.006
)

// animGateOpen reports whether ANYTHING alive still needs ticks: shimmer,
// a reveal spring in flight, an in-flight fetch riding the shared
// loadingDots phase, a picker/notifications overlay spring not yet at
// rest, either of ripple.go's two live-data windows (the status-bar
// ripple, the ✉ odometer roll), any of micro.go's three windowed effects
// (an error's shake jitter, the palette ghost's type-in, the scroll-thumb's
// fade-out — the confirm-glint needs no entry here, it rides the shimmer
// map a pending confirmation already holds open), or either of chrome.go/
// splash.go's own two (the startup splash's hold/rise, the keymap footer's
// open/close spring). No springs active ⇒ no ticks — the house rule.
func (m Model) animGateOpen() bool {
	return len(m.shimmer) > 0 || m.notif.fetching || m.pickerFetching ||
		m.entity.fetching || m.aiPicker.fetching ||
		m.rippleAnim || m.unreadOdoAnim || len(m.shaking) > 0 || m.ghostTyping || m.thumbFading ||
		m.toastTicks > 0 || m.dotPulseTicks > 0 || m.quitArmed > 0 || m.pickerDeleteArmed > 0 || m.scrollEasing || m.splashActive() || m.footerAnimating() || m.tourActive() ||
		m.aiPromptLive()
}

// aiPromptLive reports the chatbox wearing the AI accent — an @ai turn
// mid-typing on a truecolor terminal. The pulsing ">" (viewContent's
// prompt mood) needs the animation loop for as long as it's on screen,
// exactly like the web's chatbox bar animating while data-accent="ai".
func (m Model) aiPromptLive() bool {
	return m.truecolor && aiInputRe.MatchString(m.input.Value())
}

// animate ensures the animation tick loop runs while anything animGateOpen
// reports alive — shimmer, reveal springs, in-flight fetches (their
// loadingDots rows ride the shared phase), or an overlay sliding open/shut.
func (m *Model) animate() tea.Cmd {
	if m.animating || !m.animGateOpen() {
		return nil
	}
	m.animating = true
	return animTick()
}

func animTick() tea.Cmd {
	return tea.Tick(shimmerTick, func(time.Time) tea.Msg { return AnimTickMsg{} })
}

func (m Model) onAnimTick() (tea.Model, tea.Cmd) {
	if !m.animGateOpen() {
		m.animating = false
		return m, nil
	}
	m.aliveTicks++
	// ripple.go effects 1/2: two plain tick counters, each closing its own
	// window once it reaches its own budget — see rippleDurationTicks/
	// unreadOdoTicks for the exact 600ms/400ms math.
	if m.rippleAnim {
		m.rippleTick++
		if m.rippleTick >= rippleDurationTicks {
			m.rippleAnim = false
		}
	}
	if m.unreadOdoAnim {
		m.unreadOdoTick++
		if m.unreadOdoTick >= unreadOdoTicks {
			m.unreadOdoAnim = false
		}
	}
	// micro.go effect 1: advance every in-flight shake one tick, dropping
	// (never re-adding) any that just walked off errorShakeOffsets' end —
	// dirtying its turn either way so the settle frame (offset back to 0)
	// actually repaints.
	if len(m.shaking) > 0 {
		for id, sh := range m.shaking {
			sh.tick++
			m.transcript.Touch(sh.turnID)
			if sh.tick >= len(errorShakeOffsets) {
				delete(m.shaking, id)
				continue
			}
			m.shaking[id] = sh
		}
		m.pushShakeOffsets()
	} else if m.renderer != nil {
		m.renderer.SetShake(nil)
	}
	// micro.go effect 3: the ghost hint's own tick-sliced type-in.
	if m.ghostTyping {
		m.ghostTick++
		if m.ghostTick >= ghostTypeTicks {
			m.ghostTyping = false
		}
	}
	// select.go: the copied-toast's stay counts down here.
	if m.toastTicks > 0 {
		m.toastTicks--
	}
	// ambient.go effect 3: the cable-activity dot pulse breathes out.
	if m.dotPulseTicks > 0 {
		m.dotPulseTicks--
	}
	// The armed-quit window counts down (select.go's toast shape).
	if m.quitArmed > 0 {
		m.quitArmed--
	}
	// The picker's dd delete-arm window counts down the same way —
	// resume_controller.js's 500ms auto-disarm setTimeout.
	if m.pickerDeleteArmed > 0 {
		m.pickerDeleteArmed--
	}
	// micro.go effect 4: the scroll-thumb's fade-out, once follow flips
	// back to true (setFollow starts it).
	if m.thumbFading {
		m.thumbFadeTick++
		if m.thumbFadeTick >= thumbFadeTicks {
			m.thumbFading = false
		}
	}
	// splash.go effect 1 / chrome.go effect 2: the startup splash's own
	// hold/rise state machine, and the keymap footer's open/close spring.
	m.stepSplash()
	m.stepFooterAnim()
	// Visible-only animation (owner 2026-07-12: off-screen effects must
	// not cost anything): a shimmer-marked turn re-renders ONLY if its
	// lines intersect the scroller's window — and only on the SHIMMER
	// BEAT: every second tick (30fps), the owner's split-rate order
	// ("do the shimmering 30fps while keeping 60fps for the chroma").
	// The phase still advances every tick, so the sweep's wall-clock
	// speed is unchanged — half the repaints, same motion. Springs,
	// glide, sky, and every chrome effect stay on the full 60.
	if m.aliveTicks%transcriptBeatDivisor == 0 {
		winTop, winBottom := m.sc.YOffset(), m.sc.YOffset()+m.sc.height
		for turnID := range m.shimmer {
			start, end, ok := m.transcript.TurnLineRange(turnID)
			if !ok || (end > winTop && start < winBottom) {
				m.transcript.Touch(turnID)
			}
		}
	}
	// tour.go: advance --tour's own state machine one tick — typing,
	// submitting through the real Enter path, dwelling. A no-op unless
	// WithTour armed it; its command (a real send/fetch, or the
	// /notifications step's timed close) MUST ride this same batch or it
	// never actually executes.
	tourCmd := m.stepTour()
	m.phase += shimmerStep
	if m.phase >= 1 {
		m.phase -= 1
	}
	if m.renderer != nil {
		m.renderer.SetPhase(m.phase)
		// The shimmer conductor's periodic sync sweep (ambient.go) —
		// weight is 0 almost always, ramping 0→1→0 across a ~2s window
		// every ~30s of ALIVE time.
		m.renderer.SetConductor(conductorWeight(m.aliveTicks))
		// micro.go effect 2: the confirm-glint's own ~500ms sweep inside
		// its ~4s cycle — -1 (inactive) almost always, same "rides
		// aliveTicks, no dedicated gate entry" shape as the conductor.
		m.renderer.SetGlint(confirmGlintProgress(m.aliveTicks))
		// The raw counter itself, for render-side effects that key their
		// own per-element cadence off it directly — the shiny badge's
		// iridescent twinkle (render/tokens.go's sparkleActive), same
		// "rides aliveTicks, no dedicated gate entry" shape as the two
		// calls above.
		m.renderer.SetTicks(m.aliveTicks)
	}
	// The follow-glide steps every tick (60fps chrome) but needs no
	// transcript work — it only moves the scroller's offset; the window
	// materializes lazily at View time.
	if m.scrollEasing && m.follow {
		m.easeTowardBottom()
	}
	// PERF: refreshViewport (totals + clamp) only when heights can
	// actually change — reveals growing, a tour typing. Pure shimmer
	// beats just dirty their turns; scrollerView re-renders them lazily.
	if len(m.shaking) > 0 || m.tourActive() {
		m.refreshViewport()
	}
	return m, tea.Batch(animTick(), tourCmd)
}

func (m Model) onConnState(msg ConnStateMsg) (tea.Model, tea.Cmd) {
	previous := m.conn
	m.conn = msg.State
	if isUnauthorized(msg.Err) {
		m.needsLogin = true
	}
	switch msg.State {
	case cable.StateDisconnected:
		m.sawDisconnect = true
	case cable.StateConnected:
		if previous != cable.StateConnected && m.conv.UUID != "" {
			// Re-sync on EVERY confirmed subscription, first connect
			// included: events broadcast between the initial backfill and
			// the confirm are otherwise lost (live-observed: a thinking
			// resolve stuck unresolved). The merge is ID-idempotent, so
			// an extra fetch is cheap insurance.
			m.sawDisconnect = false
			return m, m.fetchChatCmd(m.conv.UUID, true)
		}
	}
	return m, nil
}

// ── View ────────────────────────────────────────────────────────────────

var (
	statusStyle = lipgloss.NewStyle().Foreground(render.ColorDim)
	warnBanner  = lipgloss.NewStyle().Background(render.ColorWarn).Foreground(render.ColorInk).Bold(true)
	errBanner   = lipgloss.NewStyle().Background(render.ColorErr).Foreground(render.ColorInk).Bold(true)
	dotOK       = lipgloss.NewStyle().Foreground(render.ColorOK).Render("■")
	dotWarn     = lipgloss.NewStyle().Foreground(render.ColorWarn).Render("■")
	dotErr      = lipgloss.NewStyle().Foreground(render.ColorErr).Render("■")
)

// View satisfies tea.Model. Bubble Tea v2's declarative View() replaces the
// v1 tea.WithAltScreen() program option — AltScreen now lives on the
// returned tea.View, set here once rather than scattered across
// NewProgram/Init. The actual content string is built by viewContent, kept
// separate so tests (and golden frames) can assert on the string directly.
func (m Model) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	// Owner 2026-07-12: the scrollbar is wheel-aware, and selection is
	// the app's own (select.go): drag selects + auto-copies with a toast,
	// Claude-Code-style, so losing the terminal's native selection to
	// mouse mode costs nothing (shift+drag still reaches it). CellMotion
	// is the narrowest v2 mode that reports both wheel and drag events.
	// Lists stay non-navigable — clicks only ever anchor selections.
	v.MouseMode = tea.MouseModeCellMotion
	if oscTitleEnabled {
		// chrome.go effect 3: a plain tea.View field in Bubble Tea v2 (not
		// a Cmd) — the renderer itself only emits the OSC sequence when
		// this string actually changes frame to frame, so recomputing it
		// every View() call is safe and needs no separate staleness check
		// here (see windowTitle's own doc comment).
		v.WindowTitle = m.windowTitle()
	}
	return v
}

func (m Model) viewContent() string {
	if !m.ready {
		return "loading…"
	}
	if m.splashActive() {
		// splash.go effect 1: pure chrome over the loading beat — replaces
		// the whole frame while active, never gating the fetches Init()
		// already fired in parallel. The star sky drifts behind the logo.
		body := m.splashView()
		if starSkyEnabled && m.skyOn && m.truecolor {
			body = paintStarSky(body, m.height, m.width, m.skyPhase)
		}
		return body
	}
	if m.mode == modePicker {
		// The picker is an overlay body — content, capped like every
		// other surface (height/window math stays on the raw terminal).
		body := pickerView(m.rows, m.cursor, m.contentWidth(), m.height, m.now(), m.truecolor, m.phase, m.pickerFetching, m.conv.UUID != "",
			m.pickerRenaming, m.pickerRenameInput.View(), m.pickerDeleteArmed > 0)
		if m.pickerNotice != "" {
			body += "\n" + statusStyle.Render("· "+m.pickerNotice)
		}
		if m.loadErr != "" {
			body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(m.loadErr)
		}
		return body
	}
	if m.mode == modeNotifications {
		return m.notificationsView()
	}
	if m.mode == modeCommandPalette {
		return m.ctrlKView()
	}
	if m.mode == modeEntityPicker {
		return m.entityPickerView()
	}
	if m.mode == modeAiPicker {
		return m.aiPickerView()
	}
	if m.mode == modeFootage {
		return m.footageView()
	}
	if m.mode == modeWarn {
		return m.warnView()
	}

	// The suggestions palette paints OVER the conversation's bottom
	// lines (owner 2026-07-12: "autocomplete shifts the conversation up
	// and down… make it over the conversation") — the viewport keeps its
	// full height and the transcript never reflows; the palette masks
	// the lines it covers, web-modal style.
	vpBody := m.scrollerView()
	if palette := m.paletteView(); palette != "" {
		vpBody = overlayBottom(vpBody, palette, m.contentWidth())
	}
	if top, bottom := m.scrollNavPills(); top != "" || bottom != "" {
		// scrollnav.go: the floating "N messages above/below" pills with
		// their ctrl+home/ctrl+end jump tokens (web's scroll-nav).
		vpBody = paintScrollNavOverlay(vpBody, top, bottom, m.contentWidth())
	}
	sections := []string{vpBody}
	if footer := m.keymapFooterView(); footer != "" {
		sections = append(sections, footer)
	}
	if banner := m.bannerLine(); banner != "" {
		sections = append(sections, banner)
	}
	if meter := m.meterLine(); meter != "" {
		sections = append(sections, meter)
	}
	if caption := m.tourCaptionView(); caption != "" {
		// tour.go: the caption card sits directly above the input — the
		// same "quiet chrome above the prompt" slot scopeHintLine and the
		// keymap footer already use.
		sections = append(sections, caption)
	}
	// The prompt wears the AI accent while an @ai turn is being typed —
	// the web tints its chatbox segment bar the same way (2.0.0).
	istyles := m.input.Styles()
	if aiInputRe.MatchString(m.input.Value()) {
		prompt := aiPromptStyle
		if m.truecolor {
			// The web's chatbox bar ANIMATES while data-accent="ai"
			// (application.css's pito-ai-bar-shimmer) — here the ">"
			// pulses purple↔pito-blue on the shared phase at
			// AIPulseSpeed (owner 2026-07-13: "make the pulses faster").
			prompt = prompt.Foreground(render.AIPromptColor(m.phase))
		}
		istyles.Focused.Prompt = prompt
		istyles.Blurred.Prompt = prompt
	} else {
		istyles.Focused.Prompt = defaultPromptStyle
		istyles.Blurred.Prompt = defaultPromptStyle
	}
	m.input.SetStyles(istyles)
	sections = append(sections, m.input.View(), m.statusLine())
	body := strings.Join(sections, "\n")
	if scrollThumbEnabled && m.truecolor && (!m.follow || m.thumbFading) {
		// micro.go effect 4: painted BEFORE the star field so a star that
		// would otherwise land on the exact same gutter column simply
		// resumes past whatever width the thumb pass just added — see
		// paintScrollThumbOverlay's own doc comment.
		body = m.paintScrollThumbOverlay(body)
	}
	if marginStarfieldEnabled && m.truecolor {
		// Post-process pass on the assembled frame — never touches how
		// any section built its own content (see ambient.go). The input
		// and status lines are always the final two entries appended
		// just above, so paintMarginStars's own "last two lines" rule
		// stays correct no matter what else this frame carries.
		body = paintMarginStars(body, m.contentWidth(), m.width, m.phase)
	}
	if starSkyEnabled && m.skyOn && m.truecolor {
		// ambient.go: the moving star sky claims the BLANK rows of the
		// conversation region (short transcripts leave a void; the sky
		// fills it, drifting on its own slow loop).
		body = paintStarSky(body, m.chatViewportHeight(), m.width, m.skyPhase)
	}
	if m.selecting {
		// select.go: the in-flight drag's reverse-video highlight — last,
		// so it wins over every other overlay under the pointer.
		body = m.paintSelectionOverlay(body)
	}
	if m.toastTicks > 0 {
		body = m.paintToastOverlay(body)
	}
	return body
}

// meterLine draws the context meter on the chatbox's top edge, exactly
// like the web: conversation name left (only when named), the thin
// gradient bar, the percent counter right. Server-computed — the TUI
// only renders what the serializer/cable said.
func (m Model) meterLine() string {
	if m.meterCtx == nil {
		return ""
	}
	name := ""
	if m.conv.DisplayName != "" && !strings.HasPrefix(m.conv.DisplayName, "Unnamed") {
		name = m.conv.DisplayName + " "
	}
	counter := " " + strconv.Itoa(int(m.meterCtx.Pct)) + "%"
	barWidth := m.contentWidth() - lipgloss.Width(name) - lipgloss.Width(counter) - 1
	if barWidth < 10 {
		barWidth = 10
	}
	return statusStyle.Render(name) +
		m.renderer.ContextMeter(m.meterCtx.Pct, barWidth) +
		statusStyle.Render(counter)
}

func (m Model) bannerLine() string {
	switch {
	case m.needsLogin:
		return warnBanner.Width(m.contentWidth()).Render(" ⚠ not logged in — send /login <code> to authenticate")
	case m.sawDisconnect && m.conn != cable.StateConnected:
		// Only after an actual drop — the initial connect is not an outage
		// (the status line already says "connecting").
		return errBanner.Width(m.contentWidth()).Render(" ⚠ disconnected — reconnecting…")
	default:
		return ""
	}
}

const paletteMax = 6

// paletteView renders the server-driven suggestion menu above the prompt
// (the web's ctrl+k palette, inlined). Selected row wears the accent bar.
func (m Model) paletteView() string {
	if m.suggest == nil || len(m.suggest.MenuItems) == 0 || m.mode != modeChat {
		return ""
	}
	items := m.suggest.MenuItems
	start := 0
	if m.suggestSel >= paletteMax {
		start = m.suggestSel - paletteMax + 1
	}
	end := min(start+paletteMax, len(items))

	labelWidth := 0
	for _, it := range items[start:end] {
		if w := lipgloss.Width(it.Label); w > labelWidth {
			labelWidth = w
		}
	}
	sel := lipgloss.NewStyle().Foreground(render.ColorAccent).Bold(true)
	var b strings.Builder
	for i := start; i < end; i++ {
		it := items[i]
		marker, label := "  ", it.Label
		if i == m.suggestSel {
			marker = sel.Render("▌ ")
			label = sel.Render(it.Label)
		}
		pad := strings.Repeat(" ", labelWidth-lipgloss.Width(it.Label))
		line := marker + label + pad
		if it.Description != "" {
			line += statusStyle.Render("  " + it.Description)
		}
		b.WriteString(lipgloss.NewStyle().MaxWidth(m.contentWidth()).Render(line) + "\n")
	}
	footer := statusStyle.Render("tab complete · ↑/↓ move · esc dismiss")
	if hint := m.suggest.Ghost.NextHint; hint != "" {
		// micro.go effect 3: types itself in left-to-right while ghostTick
		// is still mid-flight; a settled (or off-truecolor) hint renders
		// whole, same as before this effect existed.
		footer = statusStyle.Render(m.ghostDisplay(hint)+" · ") + footer
	}
	b.WriteString(footer)
	return b.String()
}

// periods mirrors the web's hardcoded cycle list (three literal copies
// in pito; this is the fourth, same contract).
var periods = []string{"7d", "28d", "3m", "1y", "lifetime"}

// hintMode decides which scope hint is live for the typed text — the
// exact port of chatbox_hints_controller.js #mode(): first token is the
// verb; analyze family → period; list/ls + any vids/games noun → channel.
func hintMode(input string) string {
	text := strings.ToLower(strings.TrimSpace(input))
	if text == "" {
		return ""
	}
	tokens := strings.Fields(text)
	verb := tokens[0]
	switch verb {
	case "analyze", "analytics", "stats":
		return "period"
	case "list", "ls":
		for _, t := range tokens[1:] {
			switch t {
			case "vid", "vids", "video", "videos", "game", "games", "gamez":
				return "channel"
			}
		}
	}
	return ""
}

// cycleNext is the web's #cycleNext verbatim: forward-only, wraps; an
// unknown current value counts as index 0 (so the next stop is list[1]).
func cycleNext(list []string, current string) string {
	if len(list) == 0 {
		return current
	}
	idx := 0
	for i, v := range list {
		if v == current {
			idx = i
			break
		}
	}
	return list[(idx+1)%len(list)]
}

// scopeHintLine renders the contextual cycler hint above the prompt,
// 1:1 with the web's chatbox_hints_controller.js + FilterComponent: a
// quiet kbd chip with the shortcut, a gap, and the current value as a
// plain token (scope chips no longer shimmer — pito 17.4). No prefix
// words — the web shows none. Channels absent → the web's own red
// "none" (pito.shell.chatbox.no_channels via copygen). The verb sets
// mirror the controller's LIST_VERBS/ANALYZE_VERBS gate (hintMode).
// One deliberate divergence: the period chip is labeled ctrl+space —
// terminals cannot report the web's shift+space, and the chip must
// name the key that actually works here.
func (m Model) scopeHintLine() string {
	value := lipgloss.NewStyle().Foreground(render.ColorAccent)
	switch hintMode(m.input.Value()) {
	case "channel":
		chip := render.Kbd(render.PitoCopy.Shell.ChannelShortcut, m.truecolor)
		if len(m.channels) == 0 {
			return chip + " " + lipgloss.NewStyle().Foreground(render.ColorErr).Render(render.PitoCopy.Shell.NoChannels)
		}
		return chip + " " + value.Render(m.scopeChannel)
	case "period":
		return render.Kbd("ctrl+space", m.truecolor) + " " + value.Render(m.scopePeriod)
	}
	return ""
}

// patchScopeCmd persists the cycled scope, fire-and-forget (the web's
// PATCH /chat/:uuid — errors only warn).
func (m Model) patchScopeCmd() tea.Cmd {
	if m.conv.UUID == "" {
		return nil
	}
	client, uuid, ch, pd := m.client, m.conv.UUID, m.scopeChannel, m.scopePeriod
	return func() tea.Msg {
		_ = client.PatchScope(context.Background(), uuid, ch, pd)
		return nil
	}
}

// helpLine is the '?'-toggled keymap strip (keybinding/hint's cousin).
func (m Model) helpLine() string {
	key := lipgloss.NewStyle().Foreground(render.ColorDim).Bold(true)
	dim := statusStyle
	pairs := []struct{ k, v string }{
		{"j/k", "scroll"}, {"shift-↑/↓", "scroll (while typing too)"},
		{"ctrl-d/u", "half page"}, {"g/G", "top/bottom"},
		{"enter", "send"}, {"?", "help"}, {"ctrl-c", "quit"},
	}
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, key.Render(p.k)+dim.Render(" "+p.v))
	}
	return dim.Render(" ") + strings.Join(parts, dim.Render(" · "))
}

// statusLine mirrors the web's right-aligned mini status: presence dot,
// host, conversation, connection state, version.
func (m Model) statusLine() string {
	// The dot law (owner 2026-07-12): RED whenever the session is
	// unauthenticated, no matter what the cable is doing; ORANGE for an
	// authenticated session still connecting (or dropped); GREEN once
	// the subscription is live — and the green one takes a single breath
	// per delivered cable message (activityPulseDot), never at idle.
	dot := dotWarn
	switch {
	case m.needsLogin:
		dot = dotErr
	case m.cableStarted && m.conn == cable.StateConnected:
		dot = dotOK
		if cableActivityPulseEnabled && m.truecolor && m.dotPulseTicks > 0 {
			dot = m.activityPulseDot()
		}
	}
	// The identity after the dot is the SERVER's build tag, nothing
	// else — pito's fat-cut 2026-07-12: "dot %{tag}", no nickname, no
	// host. Unauthenticated sessions read as pito's own anonymous word
	// (the web's "tarnished"); dev hosts read "dev" without asking.
	identity := m.resolvedServerTag()
	if m.needsLogin {
		identity = render.PitoCopy.Shell.Anonymous
	}
	// The status line's pieces, in left-to-right reading order. Built as a
	// slice rather than straight concatenation — ripple.go's effect 1 (the
	// ~600ms status-bar ripple) needs each separator's own column to paint
	// its traveling window, and every piece here is already fully
	// rendered before joining, so the whole line's width (and each
	// separator's column within it) falls out of a single forward pass —
	// see joinWithRippleSeparators. Outside a ripple this produces
	// byte-identical output to the old direct concatenation.
	pieces := make([]string, 0, 3)
	dotPiece := dot
	if identity != "" {
		dotPiece += " " + statusStyle.Render(identity)
	}
	pieces = append(pieces, dotPiece)
	if !m.needsLogin {
		// ripple.go effect 2: displayUnread is the live-interpolated ✉
		// value while a roll is in flight, the settled m.unread otherwise.
		// The slot is always present once authenticated (owner
		// 2026-07-12: an empty slot read as broken) — dim at zero, warn
		// with mail waiting.
		display := m.displayUnread()
		style := statusStyle
		if display > 0 {
			style = lipgloss.NewStyle().Foreground(render.ColorWarn)
		}
		// ⚑ (owner ruling 2026-07-12): the ✉ envelope tofu'd in his font.
		pieces = append(pieces, style.Render("⚑ "+strconv.Itoa(display)))
	}

	line := m.joinWithRippleSeparators(pieces)
	// The cycler hints live on THIS row's left side (owner 2026-07-12:
	// "use the space below the chat box, like on the same line with the
	// status bar"); the right side stays right-aligned to the content
	// column. The copied-toast overlay paints over the same left slot
	// when live — transient beats contextual for its 2.4s.
	left := m.scopeHintLine()
	if m.quitArmed > 0 {
		// The armed window: the chip brightens and asks for the second
		// press (chrome text, the footer's own vocabulary).
		left = render.Kbd("ctrl+c", m.truecolor) + " " +
			lipgloss.NewStyle().Foreground(render.ColorWarn).Bold(true).Render("again to quit")
	}
	if left == "" {
		// The way out, always in sight (owner 2026-07-12: "how do I
		// exit? We should put that on the status bar") — the contextual
		// cycler hint borrows this slot while it shows; "Quit" is the
		// keymap footer's own label for the same binding. Cached: the
		// chip is constant and lipgloss renders are not free at 60fps.
		left = quitChip(m.truecolor)
	}
	if !m.needsLogin {
		// Action affordances cluster at the right's own tail end — owner
		// ruling 2026-07-13: "action affordances cluster right, mirroring
		// pito web's ctrl+k placement; the exit affordance stays
		// separated." pito's mini_status_component.html.erb appends its
		// ctrl+k hint to the SAME right-aligned row as the auth dot/
		// notifications (both gated on @state, authenticated only) — this
		// mirrors that placement and that gate. Plain statusStyle middot,
		// deliberately OUTSIDE joinWithRippleSeparators/pieces above: the
		// ripple is a live-data reaction to a conversation.update landing
		// on the cable (ripple.go), and these two hints never react to
		// one — a static tail, not another animated piece.
		//
		// Graceful truncation (the right cluster's own, per the brief):
		// the hints are the FIRST thing to drop as the bar narrows — left
		// (the quit chip, or the armed "again to quit" warning) always
		// wins the room over decorative hints. Try the line WITH hints
		// first; fall back to the hint-less line if that doesn't fit
		// alongside left, exactly reusing the pad math below.
		withHints := line + statusStyle.Render(" · ") + actionHints(m.truecolor)
		if pad := m.contentWidth() - lipgloss.Width(left) - lipgloss.Width(withHints) - 1; pad > 0 {
			line = withHints
		}
	}
	if pad := m.contentWidth() - lipgloss.Width(left) - lipgloss.Width(line) - 1; pad > 0 {
		return left + strings.Repeat(" ", pad) + line
	}
	if pad := m.contentWidth() - lipgloss.Width(line) - 1; pad > 0 {
		return strings.Repeat(" ", pad) + line
	}
	return line
}

// transcriptBeatDivisor: the shimmer/transcript repaint runs every Nth
// 16ms tick — 2 ⇒ 30fps for text repaints while springs/glide/sky/chrome
// keep the full 60 (owner 2026-07-12).
const transcriptBeatDivisor = 2

// quitArmTicks: how long the armed-quit window stays open (~2s).
const quitArmTicks = int64(2 * time.Second / shimmerTick)

// pickerDeleteArmTicks: how long the picker's dd delete-arm window stays
// open — mirrors resume_controller.js's #arm exactly (500ms, the JS
// setTimeout that auto-disarms a lone `d`).
const pickerDeleteArmTicks = int64(500 * time.Millisecond / shimmerTick)

// quitChip memoizes the constant ctrl+c hint per color mode — statusLine
// runs every frame and the glossy Kbd chip is ~8 styled runes.
var quitChipCache [2]string

func quitChip(truecolor bool) string {
	idx := 0
	if truecolor {
		idx = 1
	}
	if quitChipCache[idx] == "" {
		quitChipCache[idx] = render.Kbd("ctrl+c", truecolor) + " " + statusStyle.Render("Quit")
	}
	return quitChipCache[idx]
}

// actionHintsCache memoizes the constant ctrl+k/ctrl+f cluster per color
// mode — statusLine runs every frame and rebuilding two styled kbd chips
// from scratch on each tick is exactly the cost quitChipCache already
// exists to dodge.
var actionHintsCache [2]string

// actionHints is the right cluster's own action-affordance tail: "ctrl+k
// commands · ctrl+f update footage" (owner ruling 2026-07-13). Each hint
// is KbdBare (no bed) + a space + the dim label — the aipicker footer's
// own shape (aipicker.go's "↑/↓ choose · enter select/connect · …") and
// scrollnav.go's scrollNavPill, not quitChip's bedded Kbd: this cluster
// rides the status line's own plain ground, not a tinted tile, matching
// how every OTHER hint row in this repo (not just the lone quit chip)
// renders its chips. ctrl+f itself opens the footage flow — see footage.go.
func actionHints(truecolor bool) string {
	idx := 0
	if truecolor {
		idx = 1
	}
	if actionHintsCache[idx] == "" {
		actionHintsCache[idx] = render.KbdBare("ctrl+f", truecolor) + " " + statusStyle.Render("update footage") +
			statusStyle.Render(" · ") +
			render.KbdBare("ctrl+k", truecolor) + " " + statusStyle.Render("commands")
	}
	return actionHintsCache[idx]
}

// resolvedServerTag is the tag after the status dot: the literal "dev"
// for dev.pitomd.com/localhost (pito's own Rails-env rule, mirrored by
// host since the TUI can't see the env), otherwise whatever GET /version
// reported (empty until it lands — the dot stands alone).
func (m Model) resolvedServerTag() string {
	if m.client == nil {
		return ""
	}
	if isDevHost(m.client.BaseURL().Hostname()) {
		return "dev"
	}
	return m.serverTag
}

// isDevHost mirrors pito's Rails-env rule by the only signal the TUI
// has: dev.pitomd.com and local hosts ARE development.
func isDevHost(host string) bool {
	return host == "dev.pitomd.com" || host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// ── Helpers ─────────────────────────────────────────────────────────────

func (m *Model) refreshViewport() {
	if !m.ready {
		return
	}
	m.sc.SetHeight(m.chatViewportHeight())
	m.sc.SetWidth(m.contentWidth())
	// Virtualized (owner 2026-07-12, the launch-guarding perf fix): no
	// full-transcript join, no full re-measure — the scroller keeps
	// TOTALS (per-turn line caches, Transcript.TotalLines) and
	// scrollerView materializes only the visible window each frame.
	transcriptLines := m.transcript.TotalLines(m.contentWidth())
	var tail []string
	if transcriptLines == 0 {
		badge := lipgloss.NewStyle().Foreground(render.ColorWarn).Bold(true).Render("[!]")
		tip := lipgloss.NewStyle().Foreground(render.ColorPrimary).Bold(true).Render(" TIP ")
		tail = append(tail, badge+tip+statusStyle.Render("— say something; the server knows the grammar. /help lists it."))
	}
	if len(m.pending) > 0 {
		// Web parity: the send window shows ONLY the comet
		// (post_command_dots has no text beside it) — the server's own
		// thinking event brings the braille spinner and the cycling verb
		// the moment it lands (render/thinking.go). No spacer entries:
		// the transcript's own trailing blank line (every event render
		// ends "\n") already separates the tail, byte-identical to the
		// old joined-string noticeSpacer semantics.
		tail = append(tail, m.spin.View())
	}
	if m.renderer != nil {
		for _, n := range m.notices {
			notice := strings.TrimRight(m.renderer.Notice(n), "\n")
			tail = append(tail, strings.Split(notice, "\n")...)
		}
	}
	m.scTail = tail
	m.sc.total = transcriptLines + len(tail)
	m.sc.clamp()
	if m.follow {
		m.easeTowardBottom()
	}
}

// easeTowardBottom replaces the old snap-to-bottom while following:
// content growth glides the viewport down over a few animation ticks
// instead of jumping (owner 2026-07-12: cable arrivals "snap and jump
// all over the place"). Small growth eases ~35%-of-the-remaining-gap a
// tick (25fps ⇒ ~150-250ms glides); anything bigger than two screens —
// the initial backfill, a resync — still lands instantly, a glide from
// nowhere would read as scrolling theater.
func (m *Model) easeTowardBottom() {
	bottom := m.sc.TotalLineCount() - m.sc.VisibleLineCount()
	if bottom < 0 {
		bottom = 0
	}
	gap := bottom - m.sc.YOffset()
	switch {
	case gap <= 0:
		m.sc.GotoBottom() // clamp (content may have SHRUNK under us)
		m.scrollEasing = false
	case gap > 2*m.sc.VisibleLineCount():
		m.sc.GotoBottom()
		m.scrollEasing = false
	default:
		step := (gap*18 + 99) / 100 // ceil(gap*0.18) at 60fps ≈ the old 35%/25fps feel
		m.sc.SetYOffset(m.sc.YOffset() + step)
		m.scrollEasing = m.sc.YOffset() < bottom
	}
}

func (m *Model) pushNotice(text string) {
	m.notices = append(m.notices, text)
	if len(m.notices) > maxNotices {
		m.notices = m.notices[len(m.notices)-maxNotices:]
	}
}

// contentWidth is the owner-locked containment law (v2.1.0
// "containment"): conversation content — message blocks, cards, tables,
// chart canvases, banners, the palette, overlay bodies — renders
// LEFT-ANCHORED inside min(terminalWidth−2, render.ContentCap) columns,
// never stretching across a huge terminal. Since the owner's 2.0.0
// smoke (2026-07-12) the prompt and status bar follow the SAME column —
// the app is one coherent column on wide terminals, the margin belongs
// to the star-field.
//
// Below the cap's bite point (terminalWidth ≤ ContentCap+2) this
// returns the raw terminal width, unmargined — today's narrow-terminal
// rendering, 80×24 goldens included, never moves. The −2 margin only
// takes effect once it would otherwise leave MORE than ContentCap
// columns, so the cap bites strictly above ContentCap+2.
func (m Model) contentWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	// Owner 2026-07-12 (final ruling after smoking both looks): NO max
	// width — the app uses the full terminal. The cap machinery stays
	// one flag away (widthCapEnabled) in case wide-terminal reading
	// comfort ever wins him back; the coherent-column chrome work keeps
	// everything sized off THIS function either way.
	if widthCapEnabled {
		if margined := w - 2; margined > render.ContentCap {
			return render.ContentCap
		}
	}
	return w
}

// chatViewportHeight is the terminal height minus prompt + status, the
// banner, and the palette when they show.
func (m Model) chatViewportHeight() int {
	h := m.height - 2
	if m.meterCtx != nil {
		h--
	}
	if m.bannerLine() != "" {
		h--
	}
	if m.keymapFooterView() != "" {
		h--
	}
	if m.tourCaptionView() != "" {
		h -= 2 // tour.go: the caption card is a text line plus its rule
	}
	if h < 1 {
		h = 1
	}
	return h
}

func isUnauthorized(err error) bool {
	return errors.Is(err, api.ErrUnauthorized)
}
