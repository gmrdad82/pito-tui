// Package ui is the Bubble Tea program: one model covering the
// conversation picker and the chat screen. It mirrors the web shell —
// scrollback of turn blocks, one prompt, slash commands passed through as
// raw text. The server grammar stays authoritative; nothing is parsed here.
package ui

import (
	"context"
	"errors"
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

	mode        mode
	plainRender bool
	renderer    *render.R

	// picker state
	rows   []pickerRow
	cursor int
	now    func() time.Time

	// chat state
	vp             viewport.Model
	input          textinput.Model
	spin           spinner.Model
	transcript     *Transcript
	conv           api.Conversation
	conn           cable.ConnState
	sawDisconnect  bool
	cableStarted   bool
	pending        map[int64]bool
	follow         bool
	notices        []string
	sessionExpired bool
	loadErr        string

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

// WithPlainRender disables glamour/color variance for golden tests.
func WithPlainRender() Option {
	return func(m *Model) { m.plainRender = true }
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

func NewModel(client *api.Client, connect ConnectFunc, opts ...Option) Model {
	input := textinput.New()
	input.Placeholder = "message or /command"
	input.Prompt = "> "
	input.Focus()

	spin := spinner.New(spinner.WithSpinner(spinner.MiniDot))

	m := Model{
		client:  client,
		connect: connect,
		sounds:  noopNotifier{},
		mode:    modePicker,
		input:   input,
		spin:    spin,
		pending: map[int64]bool{},
		follow:  true,
		now:     time.Now,
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

func (m Model) sendCmd(uuid, input string, width int) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		res, err := client.SendMessage(context.Background(), uuid, input, width)
		return SendResultMsg{Res: res, Err: err}
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
		return m.onCableEvent(msg), nil
	case ConnStateMsg:
		return m.onConnState(msg)
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
	switch msg.Type {
	case tea.KeyEnter:
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		m.input.Reset()
		m.sounds.Send()
		return m, m.sendCmd(m.conv.UUID, text, m.contentWidth())
	case tea.KeyCtrlD:
		m.vp.HalfPageDown()
		m.follow = m.vp.AtBottom()
		return m, nil
	case tea.KeyCtrlU:
		m.vp.HalfPageUp()
		m.follow = m.vp.AtBottom()
		return m, nil
	}

	// Vim-style scrolling only while the prompt is empty — otherwise every
	// key belongs to the text input (no focus toggle to learn).
	if m.input.Value() == "" {
		switch msg.String() {
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

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) onResume(msg ResumeFetchedMsg) Model {
	if msg.Err != nil {
		m.loadErr = "could not load conversations: " + msg.Err.Error()
		return m
	}
	m.rows = pickerRows(msg.List)
	m.loadErr = ""
	return m
}

func (m Model) onChatFetched(msg ChatFetchedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		if isUnauthorized(msg.Err) {
			m.sessionExpired = true
		} else {
			m.pushNotice("scrollback fetch failed: " + msg.Err.Error())
		}
		m.refreshViewport()
		return m, nil
	}
	m.conv = msg.Page.Conversation
	m.transcript.Merge(msg.Page.Events)
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
	return m, nil
}

func (m Model) onSendResult(msg SendResultMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		if isUnauthorized(msg.Err) {
			m.sessionExpired = true
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
		if res.TurnID != 0 {
			m.pending[res.TurnID] = true
		}
		return m, tea.Batch(m.fetchChatCmd(res.CreatedUUID, false), m.spin.Tick)
	default:
		m.pending[res.TurnID] = true
		m.refreshViewport()
		return m, m.spin.Tick
	}
}

func (m Model) onCableEvent(msg CableEventMsg) Model {
	ev := msg.M.Event
	switch msg.M.Type {
	case cable.TypeEventAppend:
		if m.pending[ev.TurnID] {
			delete(m.pending, ev.TurnID)
			m.sounds.Receive()
		}
		m.transcript.Append(ev)
	case cable.TypeEventReplace:
		m.transcript.Replace(ev)
	default:
		return m // unknown stream message type: ignore
	}
	m.refreshViewport()
	return m
}

func (m Model) onConnState(msg ConnStateMsg) (tea.Model, tea.Cmd) {
	previous := m.conn
	m.conn = msg.State
	if isUnauthorized(msg.Err) {
		m.sessionExpired = true
	}
	switch msg.State {
	case cable.StateDisconnected:
		m.sawDisconnect = true
	case cable.StateConnected:
		if m.sawDisconnect && previous != cable.StateConnected {
			// The cable has no replay: refetch and diff-merge.
			m.sawDisconnect = false
			return m, m.fetchChatCmd(m.conv.UUID, true)
		}
	}
	return m, nil
}

// ── View ────────────────────────────────────────────────────────────────

var (
	statusStyle  = lipgloss.NewStyle().Faint(true)
	bannerStyle  = lipgloss.NewStyle().Reverse(true).Bold(true)
	noticeSpacer = "\n"
)

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	if m.mode == modePicker {
		body := pickerView(m.rows, m.cursor, m.width, m.now())
		if m.loadErr != "" {
			body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(m.loadErr)
		}
		return body
	}

	sections := []string{m.vp.View()}
	if banner := m.bannerLine(); banner != "" {
		sections = append(sections, banner)
	}
	sections = append(sections, m.input.View(), m.statusLine())
	return strings.Join(sections, "\n")
}

func (m Model) bannerLine() string {
	switch {
	case m.sessionExpired:
		return bannerStyle.Width(m.width).Render("⚠ session expired — restart pito-tui to log in again")
	case m.sawDisconnect && m.conn != cable.StateConnected:
		// Only after an actual drop — the initial connect is not an outage
		// (the status line already says "connecting").
		return bannerStyle.Width(m.width).Render("⚠ disconnected — reconnecting…")
	default:
		return ""
	}
}

func (m Model) statusLine() string {
	dot := "●"
	state := m.conn.String()
	if !m.cableStarted {
		state = "not connected"
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
	parts := []string{dot + " " + state, name}
	if host != "" {
		parts = append(parts, host)
	}
	parts = append(parts, version.String())
	return statusStyle.Width(m.width).Render(strings.Join(parts, " · "))
}

// ── Helpers ─────────────────────────────────────────────────────────────

func (m *Model) refreshViewport() {
	if !m.ready {
		return
	}
	m.vp.Height = m.chatViewportHeight()
	content := m.transcript.View(m.contentWidth())
	if content == "" {
		content = statusStyle.Render("── no messages yet — say something")
	}
	if len(m.pending) > 0 {
		content += noticeSpacer + m.spin.View() + " thinking…"
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

// chatViewportHeight is the terminal height minus prompt + status and the
// banner when one is showing.
func (m Model) chatViewportHeight() int {
	h := m.height - 2
	if m.bannerLine() != "" {
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
