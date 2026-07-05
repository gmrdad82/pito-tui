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

// ImageDisplay pins one image to the terminal (kitty graphics). nil on
// terminals without image support — everything degrades to text.
type ImageDisplay interface {
	Show(data []byte, row, col, cols, rows int)
	Clear()
}

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
	images  ImageDisplay

	mode         mode
	plainRender  bool
	glamourStyle string
	truecolor    bool
	renderer     *render.R
	shimmer      map[int64]time.Time // turnID → when its shimmer went live
	phase        float64
	animating    bool

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
	loadErr       string
	pickerNotice  string

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

// WithImages wires the terminal image display (kitty graphics).
func WithImages(d ImageDisplay) Option {
	return func(m *Model) { m.images = d }
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
		client:  client,
		connect: connect,
		sounds:  noopNotifier{},
		mode:    modePicker,
		input:   input,
		spin:    spin,
		pending: map[int64]bool{},
		shimmer: map[int64]time.Time{},
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
		return m.onCableEvent(msg)
	case ConnStateMsg:
		return m.onConnState(msg)
	case AnimTickMsg:
		return m.onAnimTick()
	case ImageFetchedMsg:
		if m.images != nil && m.width > 46 {
			// Pin to the top-right corner, clear of the scrollback's left
			// column; kitty scales into the cell box.
			m.images.Show(msg.Data, 2, m.width-40, 38, 11)
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
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
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
	m.needsLogin = false // the backfill is auth-gated: fetching it proves the session
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
			// turn_id is null for reply mutations (confirmation replies):
			// nothing new is pending, the replace arrives on the cable.
			return m, nil
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
			m.shimmer[ev.TurnID] = time.Now()
		}
	case cable.TypeEventReplace:
		// A replace proves the turn is flowing too — the thinking
		// resolve (event.replace, resolved: true) is THE turn-done
		// signal per the contract, so pending must not survive it.
		delete(m.pending, ev.TurnID)
		m.transcript.Replace(ev)
	default:
		return m, nil // unknown stream message type: ignore
	}
	m.refreshViewport()
	return m, tea.Batch(m.imageCmd(ev), m.animate())
}

const shimmerLife = 4400 * time.Millisecond // one web sweep cycle, twice

// animate ensures the shimmer tick loop runs while anything is fresh.
func (m *Model) animate() tea.Cmd {
	if m.animating || len(m.shimmer) == 0 {
		return nil
	}
	m.animating = true
	return animTick()
}

func animTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return AnimTickMsg{} })
}

func (m Model) onAnimTick() (tea.Model, tea.Cmd) {
	now := time.Now()
	for turnID, born := range m.shimmer {
		if now.Sub(born) > shimmerLife {
			delete(m.shimmer, turnID) // settle at the current phase
			continue
		}
		m.transcript.Touch(turnID)
	}
	if len(m.shimmer) == 0 {
		m.animating = false
		m.refreshViewport()
		return m, nil
	}
	m.phase += 0.045 // full sweep ≈ 2.2s at 10fps
	if m.phase >= 1 {
		m.phase -= 1
	}
	if m.renderer != nil {
		m.renderer.SetPhase(m.phase)
	}
	m.refreshViewport()
	return m, animTick()
}

// imageCmd fetches a message's thumbnail for the pinned display. Only on
// image-capable terminals, only for card-bearing kinds, never blocking.
func (m Model) imageCmd(ev api.Event) tea.Cmd {
	if m.images == nil {
		return nil
	}
	switch ev.Kind {
	case api.KindSystem, api.KindEnhanced, api.KindSystemFollowUp, api.KindEnhancedFollowUp:
	default:
		return nil
	}
	src := extractImageSrc(ev.Payload)
	if src == "" {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		data, err := client.FetchRaw(context.Background(), src)
		if err != nil {
			return nil
		}
		return ImageFetchedMsg{Data: data}
	}
}

// thumbnailRe finds the first non-avatar image in a card body.
var thumbnailRe = regexp.MustCompile(`<img[^>]+>`)

var srcRe = regexp.MustCompile(`src="([^"]+)"`)

func extractImageSrc(payload []byte) string {
	var p struct {
		Body string `json:"body"`
		HTML bool   `json:"html"`
	}
	if json.Unmarshal(payload, &p) != nil || !p.HTML || p.Body == "" {
		return ""
	}
	for _, tag := range thumbnailRe.FindAllString(p.Body, 4) {
		if strings.Contains(tag, "tiny-avatar") {
			continue // list avatars: too small to pin
		}
		if match := srcRe.FindStringSubmatch(tag); match != nil && strings.HasPrefix(match[1], "/") {
			return match[1]
		}
	}
	return ""
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
		body := pickerView(m.rows, m.cursor, m.width, m.height, m.now())
		if m.pickerNotice != "" {
			body += "\n" + statusStyle.Render("· "+m.pickerNotice)
		}
		if m.loadErr != "" {
			body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(m.loadErr)
		}
		return body
	}

	sections := []string{m.vp.View()}
	if m.showHelp {
		sections = append(sections, m.helpLine())
	}
	if banner := m.bannerLine(); banner != "" {
		sections = append(sections, banner)
	}
	sections = append(sections, m.input.View(), m.statusLine())
	return strings.Join(sections, "\n")
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

// helpLine is the '?'-toggled keymap strip (keybinding/hint's cousin).
func (m Model) helpLine() string {
	key := lipgloss.NewStyle().Foreground(render.ColorDim).Bold(true)
	dim := statusStyle
	pairs := []struct{ k, v string }{
		{"j/k", "scroll"}, {"ctrl-d/u", "half page"}, {"g/G", "top/bottom"},
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
	line := dot + " " + host + sep + statusStyle.Render(name) + sep +
		statusStyle.Render(state) + sep + statusStyle.Render(version.String())
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
