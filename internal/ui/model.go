// Package ui is the Bubble Tea program: one model covering the
// conversation picker and the chat screen. It mirrors the web shell —
// scrollback of turn blocks, one prompt, slash commands passed through as
// raw text. The server grammar stays authoritative; nothing is parsed here.
package ui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gmrdad82/pito-tui/internal/api"
	"github.com/gmrdad82/pito-tui/internal/cable"
	"github.com/gmrdad82/pito-tui/internal/ui/render"
	"github.com/gmrdad82/pito-tui/internal/version"
)

type mode int

const (
	modePicker mode = iota
	modeChat
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
	// (the web's pito-bar-reveal): eventID → birth + turn for dirtying.
	revealing map[int64]revealInfo
	phase     float64
	animating bool

	// picker state
	rows   []pickerRow
	cursor int
	now    func() time.Time

	// chat state
	vp            viewport.Model
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
	loadErr       string
	pickerNotice  string
	meterCtx      *api.ContextMeter // server-computed context (render only)
	me            *api.Identity
	unread        int
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

func NewModel(client *api.Client, connect ConnectFunc, opts ...Option) Model {
	input := textinput.New()
	input.Placeholder = "/help to see available commands"
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(render.ColorFaint)
	input.Prompt = "> "
	input.PromptStyle = lipgloss.NewStyle().Foreground(render.ColorAccent).Bold(true)
	input.Cursor.Style = lipgloss.NewStyle().Foreground(render.ColorAccent)
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
		revealing:    map[int64]revealInfo{},
		follow:       true,
		now:          time.Now,
		scopeChannel: "@all",
		scopePeriod:  "7d",
	}
	m.transcript = NewTranscript(nil)
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	switch {
	case m.mode == modeChat && m.conv.UUID != "":
		cmds = append(cmds, m.fetchChatCmd(m.conv.UUID, false))
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
		list, err := client.Resume(context.Background())
		return ResumeFetchedMsg{List: list, Err: err}
	}
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
		return m.onResize(msg), nil
	case tea.KeyMsg:
		return m.onKey(msg)
	case ResumeFetchedMsg:
		return m.onResume(msg), nil
	case ChatFetchedMsg:
		return m.onChatFetched(msg)
	case SendResultMsg:
		return m.onSendResult(msg)
	case CableEventMsg:
		return m.onCableEvent(msg)
	case ConnStateMsg:
		return m.onConnState(msg)
	case resyncNowMsg:
		return m, msg.cmd
	case AnimTickMsg:
		return m.onAnimTick()
	case SuggestionsMsg:
		if msg.Seq == m.suggestSeq && m.input.Value() != "" {
			m.suggest = msg.S
			m.suggestSel = 0
			m.refreshViewport() // palette height changes the viewport box
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
	opts := []render.Option{}
	if m.plainRender {
		opts = append(opts, render.WithPlain())
	}
	if m.glamourStyle != "" {
		opts = append(opts, render.WithStyle(m.glamourStyle))
	}
	opts = append(opts, render.WithTruecolor(m.truecolor))
	m.renderer = render.New(m.contentWidth(), opts...)
	renderer := m.renderer
	m.transcript.SetRenderer(func(ev api.Event, _ int) string { return renderer.Event(ev) })

	if !m.ready {
		m.vp = viewport.New(m.width, m.chatViewportHeight())
	} else {
		m.vp.Width = m.width
		m.vp.Height = m.chatViewportHeight()
	}
	m.input.Width = m.width - len(m.input.Prompt) - 1
	m.ready = true
	m.refreshViewport()
	return m
}

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	if m.mode == modePicker {
		return m.onPickerKey(msg)
	}
	return m.onChatKey(msg)
}

func (m Model) onPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.rows) == 0 {
			return m, nil
		}
		row := m.rows[m.cursor]
		m.mode = modeChat
		if row.isNew {
			// No uuid yet — the first send creates the conversation.
			return m, nil
		}
		m.conv.UUID = row.uuid
		m.conv.DisplayName = row.title
		return m, m.fetchChatCmd(row.uuid, false)
	}
	return m, nil
}

func (m Model) onChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			return m, m.suggestCmd()
		case " ":
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

	switch msg.Type {
	case tea.KeyShiftTab:
		// Channel cycler — only while its hint is live (web parity).
		if hintMode(m.input.Value()) == "channel" && len(m.channels) > 0 {
			m.scopeChannel = cycleNext(m.channels, m.scopeChannel)
			return m, m.patchScopeCmd()
		}
		return m, nil
	case tea.KeyEnter:
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
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
	case tea.KeyCtrlD:
		m.vp.HalfPageDown()
		m.follow = m.vp.AtBottom()
		return m, nil
	case tea.KeyCtrlU:
		m.vp.HalfPageUp()
		m.follow = m.vp.AtBottom()
		return m, nil
	case tea.KeyShiftUp:
		// Web parity: shift+↑/↓ scroll the conversation — and unlike the
		// vim keys they work mid-typing (they never collide with input).
		m.vp.ScrollUp(1)
		m.follow = m.vp.AtBottom()
		return m, nil
	case tea.KeyShiftDown:
		m.vp.ScrollDown(1)
		m.follow = m.vp.AtBottom()
		return m, nil
	}

	// Vim-style scrolling only while the prompt is empty — otherwise every
	// key belongs to the text input (no focus toggle to learn).
	if msg.String() == "ctrl+@" {
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
			return m, nil
		case "R":
			// Shift+R at an empty prompt (web parity, caret-0 rule):
			// prefill the newest live reply handle; several → the palette
			// becomes the picker; none → the R types through below.
			handles := m.transcript.LiveHandles()
			switch {
			case len(handles) == 1:
				m.input.SetValue("#" + handles[0] + " ")
				m.input.CursorEnd()
				m.suggest = nil
				return m, m.suggestCmd()
			case len(handles) > 1:
				items := make([]api.Suggestion, 0, len(handles))
				for _, h := range handles {
					items = append(items, api.Suggestion{Label: "#" + h, Insert: "#" + h})
				}
				m.suggest = &api.Suggestions{MenuItems: items}
				m.suggestSel = 0
				m.refreshViewport()
				return m, nil
			}
		case "j":
			m.vp.ScrollDown(1)
			m.follow = m.vp.AtBottom()
			return m, nil
		case "k":
			m.vp.ScrollUp(1)
			m.follow = m.vp.AtBottom()
			return m, nil
		case "g":
			m.vp.GotoTop()
			m.follow = false
			return m, nil
		case "G":
			m.vp.GotoBottom()
			m.follow = true
			return m, nil
		}
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		if m.input.Value() == "" {
			m.suggest = nil
			m.refreshViewport()
			return m, cmd
		}
		return m, tea.Batch(cmd, m.suggestCmd())
	}
	return m, cmd
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

func (m Model) onResume(msg ResumeFetchedMsg) Model {
	if msg.Err != nil {
		m.loadErr = "could not load conversations: " + msg.Err.Error()
		return m
	}
	m.rows = pickerRows(msg.List)
	m.needsLogin = false // resume is auth-gated too
	m.loadErr = ""
	return m
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
			return m, m.fetchResumeCmd()
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
	if msg.Page.Me != nil {
		m.me = msg.Page.Me
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
	m.transcript.Merge(msg.Page.Events)
	for _, ev := range msg.Page.Events {
		if m.truecolor && render.HasShimmer(ev.Payload) {
			m.shimmer[ev.TurnID] = true
		}
		if m.truecolor && (ev.Kind == api.KindConfirmation || ev.Kind == api.KindConfirmationFollowUp) {
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
	return m, m.animate()
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
	ev := msg.M.Event
	switch msg.M.Type {
	case cable.TypeEventAppend:
		if m.pending[ev.TurnID] {
			delete(m.pending, ev.TurnID)
			m.sounds.Receive()
		}
		m.transcript.Append(ev)
		if m.truecolor && render.HasShimmer(ev.Payload) {
			m.shimmer[ev.TurnID] = true
		}
		m.markReveal(ev)
		if m.truecolor && (ev.Kind == api.KindConfirmation || ev.Kind == api.KindConfirmationFollowUp) {
			m.shimmer[ev.TurnID] = true // the warn border pulses while live
		}
	case cable.TypeEventReplace:
		// A replace proves the turn is flowing too — the thinking
		// resolve (event.replace, resolved: true) is THE turn-done
		// signal per the contract, so pending must not survive it.
		delete(m.pending, ev.TurnID)
		m.transcript.Replace(ev)
		m.markReveal(ev) // a fill landing via replace grows in too
	case cable.TypeConversationUpdate:
		if msg.M.Context != nil {
			m.meterCtx = msg.M.Context
		}
		if msg.M.Notifications != nil {
			m.unread = msg.M.Notifications.Unread
		}
	default:
		return m, nil // unknown stream message type: ignore
	}
	m.refreshViewport()
	return m, m.animate()
}

// revealInfo tracks one freshly-arrived event growing in.
type revealInfo struct {
	turnID int64
	born   time.Time
}

// revealDuration is the web's pito-bar-reveal length, terminal-tuned.
const revealDuration = 600 * time.Millisecond

// markReveal starts the grow-in for charts/bars that just arrived live.
func (m *Model) markReveal(ev api.Event) {
	if !m.truecolor || !render.HasShimmer(ev.Payload) {
		return
	}
	m.revealing[ev.ID] = revealInfo{turnID: ev.TurnID, born: time.Now()}
}

// Shimmer runs indefinitely, like the web (owner call, 2026-07-05) — the
// tick loop lives as long as shimmer-marked turns are in the transcript.
// 40ms ticks (25fps — owner call 2026-07-06: 12.5fps read as visible
// ticking) with a ~2.7s cycle; per-element stagger lives in the
// renderer (phaseOffset), so the shared phase never syncs neighbors.
const (
	shimmerTick = 40 * time.Millisecond
	shimmerStep = 0.015
)

// animate ensures the animation tick loop runs while shimmer lives.
func (m *Model) animate() tea.Cmd {
	if m.animating || (len(m.shimmer) == 0 && len(m.revealing) == 0) {
		return nil
	}
	m.animating = true
	return animTick()
}

func animTick() tea.Cmd {
	return tea.Tick(shimmerTick, func(time.Time) tea.Msg { return AnimTickMsg{} })
}

func (m Model) onAnimTick() (tea.Model, tea.Cmd) {
	if len(m.shimmer) == 0 && len(m.revealing) == 0 {
		m.animating = false
		return m, nil
	}
	for turnID := range m.shimmer {
		m.transcript.Touch(turnID)
	}
	// Advance the grow-ins: eased fraction per revealing event; done ones
	// leave the map (they render full from then on).
	if len(m.revealing) > 0 {
		fracs := make(map[int64]float64, len(m.revealing))
		for id, info := range m.revealing {
			t := float64(time.Since(info.born)) / float64(revealDuration)
			if t >= 1 {
				delete(m.revealing, id)
				m.transcript.Touch(info.turnID)
				continue
			}
			fracs[id] = 1 - (1-t)*(1-t) // ease-out
			m.transcript.Touch(info.turnID)
		}
		if m.renderer != nil {
			m.renderer.SetReveal(fracs)
		}
	} else if m.renderer != nil {
		m.renderer.SetReveal(nil)
	}
	m.phase += shimmerStep
	if m.phase >= 1 {
		m.phase -= 1
	}
	if m.renderer != nil {
		m.renderer.SetPhase(m.phase)
	}
	m.refreshViewport()
	return m, animTick()
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
	statusStyle  = lipgloss.NewStyle().Foreground(render.ColorDim)
	warnBanner   = lipgloss.NewStyle().Background(render.ColorWarn).Foreground(render.ColorInk).Bold(true)
	errBanner    = lipgloss.NewStyle().Background(render.ColorErr).Foreground(render.ColorInk).Bold(true)
	dotOK        = lipgloss.NewStyle().Foreground(render.ColorOK).Render("■")
	dotWarn      = lipgloss.NewStyle().Foreground(render.ColorWarn).Render("■")
	dotErr       = lipgloss.NewStyle().Foreground(render.ColorErr).Render("■")
	noticeSpacer = "\n"
)

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	if m.mode == modePicker {
		body := pickerView(m.rows, m.cursor, m.width, m.height, m.now(), m.truecolor)
		if m.pickerNotice != "" {
			body += "\n" + statusStyle.Render("· "+m.pickerNotice)
		}
		if m.loadErr != "" {
			body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(m.loadErr)
		}
		return body
	}

	sections := []string{m.vp.View()}
	if palette := m.paletteView(); palette != "" {
		sections = append(sections, palette)
	}
	if m.showHelp {
		sections = append(sections, m.helpLine())
	}
	if banner := m.bannerLine(); banner != "" {
		sections = append(sections, banner)
	}
	if meter := m.meterLine(); meter != "" {
		sections = append(sections, meter)
	}
	if hint := m.scopeHintLine(); hint != "" {
		sections = append(sections, hint)
	}
	sections = append(sections, m.input.View(), m.statusLine())
	return strings.Join(sections, "\n")
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
	barWidth := m.width - lipgloss.Width(name) - lipgloss.Width(counter) - 1
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
		return warnBanner.Width(m.width).Render(" ⚠ not logged in — send /login <code> to authenticate")
	case m.sawDisconnect && m.conn != cable.StateConnected:
		// Only after an actual drop — the initial connect is not an outage
		// (the status line already says "connecting").
		return errBanner.Width(m.width).Render(" ⚠ disconnected — reconnecting…")
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
		b.WriteString(lipgloss.NewStyle().MaxWidth(m.width).Render(line) + "\n")
	}
	footer := statusStyle.Render("tab complete · ↑/↓ move · esc dismiss")
	if hint := m.suggest.Ghost.NextHint; hint != "" {
		footer = statusStyle.Render(hint+" · ") + footer
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

// scopeHintLine renders the dim cycler hint above the prompt when a
// scope verb is being typed — the web's shiftTab/shiftSpace hint row.
func (m Model) scopeHintLine() string {
	switch hintMode(m.input.Value()) {
	case "channel":
		if len(m.channels) == 0 {
			return "" // channel list not served yet (tui-needs ask pending)
		}
		return statusStyle.Render("shift+tab channel: ") +
			lipgloss.NewStyle().Foreground(render.ColorAccent).Render(m.scopeChannel)
	case "period":
		return statusStyle.Render("ctrl+space period: ") +
			lipgloss.NewStyle().Foreground(render.ColorAccent).Render(m.scopePeriod)
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
	dot := dotErr
	state := m.conn.String()
	switch {
	case !m.cableStarted:
		dot, state = dotWarn, "not connected"
	case m.conn == cable.StateConnected:
		dot = dotOK
	case m.conn == cable.StateConnecting:
		dot = dotWarn
	}
	name := m.conv.Label()
	if name == "" {
		if m.conv.UUID == "" {
			name = "new conversation"
		} else {
			name = "(unnamed)"
		}
	}
	host := ""
	if m.client != nil {
		host = m.client.BaseURL().Host
	}
	sep := statusStyle.Render(" · ")
	line := dot + " " + render.Brand(host, m.truecolor) + sep + statusStyle.Render(name) + sep +
		statusStyle.Render(state) + sep + statusStyle.Render(version.String())
	if m.me != nil {
		line = statusStyle.Render(m.me.Handle) + sep + line
	}
	if m.unread > 0 {
		line += sep + lipgloss.NewStyle().Foreground(render.ColorWarn).Render("✉ "+strconv.Itoa(m.unread))
	}
	if pad := m.width - lipgloss.Width(line) - 1; pad > 0 {
		line = strings.Repeat(" ", pad) + line
	}
	return line
}

// ── Helpers ─────────────────────────────────────────────────────────────

func (m *Model) refreshViewport() {
	if !m.ready {
		return
	}
	m.vp.Height = m.chatViewportHeight()
	content := m.transcript.View(m.contentWidth())
	if content == "" {
		badge := lipgloss.NewStyle().Foreground(render.ColorWarn).Bold(true).Render("[!]")
		tip := lipgloss.NewStyle().Foreground(render.ColorPrimary).Bold(true).Render(" TIP ")
		content = badge + tip + statusStyle.Render("— say something; the server knows the grammar. /help lists it.")
	}
	if len(m.pending) > 0 {
		line := m.spin.View() + " thinking…"
		if m.truecolor {
			line = m.spin.View() + render.PitoShimmer.Colorize(" thinking…", m.phase)
		}
		content += noticeSpacer + line
	}
	if m.renderer != nil {
		for _, n := range m.notices {
			content += noticeSpacer + strings.TrimRight(m.renderer.Notice(n), "\n")
		}
	}
	m.vp.SetContent(content)
	if m.follow {
		m.vp.GotoBottom()
	}
}

func (m *Model) pushNotice(text string) {
	m.notices = append(m.notices, text)
	if len(m.notices) > maxNotices {
		m.notices = m.notices[len(m.notices)-maxNotices:]
	}
}

func (m Model) contentWidth() int {
	if m.width <= 0 {
		return 80
	}
	return m.width
}

// chatViewportHeight is the terminal height minus prompt + status, the
// banner, and the palette when they show.
func (m Model) chatViewportHeight() int {
	h := m.height - 2
	if m.meterCtx != nil {
		h--
	}
	if m.scopeHintLine() != "" {
		h--
	}
	if m.bannerLine() != "" {
		h--
	}
	if palette := m.paletteView(); palette != "" {
		h -= strings.Count(palette, "\n") + 1
	}
	if m.showHelp {
		h--
	}
	if h < 1 {
		h = 1
	}
	return h
}

func isUnauthorized(err error) bool {
	return errors.Is(err, api.ErrUnauthorized)
}
