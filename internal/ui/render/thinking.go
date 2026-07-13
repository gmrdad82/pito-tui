// The thinking indicator, 1:1 with the web's ThinkingComponent (owner
// smoke feedback 2026-07-12: "thinking and crunching numbers seems off.
// With lower case, no shimmering like in the web. no braille indicator").
//
// Pending: a braille spinner (the web's shared FRAMES, 80ms a frame —
// two 40ms animation ticks) beside the dictionary verb the server's own
// formula picks for this moment — doing_words[order[elapsed/5s % len]] —
// with a trailing ellipsis, the whole line wearing the network shimmer
// (purple base, brand-pito band). The verbs come from pito's committed
// copy via tools/copygen (thinking_copy.json); the payload's dictionary/
// order/started_at are replayed here exactly like the web's Stimulus
// controller replays its data attributes, so the TUI and a browser
// looking at the same turn show the same word at the same second.
//
// Resolved: a kaomoji glyph seeded by the event id (the component's own
// GLYPHS pool — visual symbols, hardcoded there too) plus the past-tense
// word and elapsed time in the locale's resolved shape, "%{word} for
// %{elapsed}s", elapsed at ≤2 decimals with trailing zeros stripped.
package render

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/gmrdad82/pito-tui/internal/api"
)

//go:generate env PITO_REF=v2.0.0 go run github.com/gmrdad82/pito-tui/tools/copygen

//go:embed pito_copy.json
var pitoCopyJSON []byte

// thinkingCopy is the generated snapshot of pito's
// en.pito.copy.thinking.* word pools. Array order is load-bearing: the
// payload's order/word_index fields index into these.
// PitoCopy is the generated mirror of pito's user-facing copy pools
// (tools/copygen, pinned ref) — the COPY LAW's delivery mechanism. The
// exported shape lets the ui package read the palette/shell/start-screen
// strings directly; the thinking dictionaries stay package-internal.
type PitoCopySnapshot struct {
	Dictionaries map[string]struct {
		Doing []string `json:"doing"`
		Done  []string `json:"done"`
	} `json:"dictionaries"`
	Clipboard   []string `json:"clipboard"`
	StartScreen struct {
		TipPrefix string   `json:"tip_prefix"`
		Tips      []string `json:"tips"`
	} `json:"start_screen"`
	Palette struct {
		Title             string            `json:"title"`
		EscHint           string            `json:"esc_hint"`
		SearchPlaceholder string            `json:"search_placeholder"`
		Sections          map[string]string `json:"sections"`
		Commands          map[string]string `json:"commands"`
	} `json:"palette"`
	// AiPicker is the /config ai model picker's chrome
	// (en.pito.palette.ai_picker + en.pito.copy.ai.picker.key_gate).
	AiPicker struct {
		Title             string            `json:"title"`
		EscHint           string            `json:"esc_hint"`
		SearchPlaceholder string            `json:"search_placeholder"`
		NoModel           string            `json:"no_model"`
		Sections          map[string]string `json:"sections"`
		KeyGate           string            `json:"key_gate"`
	} `json:"ai_picker"`
	// ScrollbackNav is the floating scroll-pill copy
	// (en.pito.copy.scrollback_nav): one fixed string per side
	// (%{count} the only token) and the two jump glyphs.
	ScrollbackNav struct {
		Before      string `json:"before"`
		After       string `json:"after"`
		JumpToStart string `json:"jump_to_start"`
		JumpToEnd   string `json:"jump_to_end"`
	} `json:"scrollback_nav"`
	Shell struct {
		ChannelShortcut string `json:"channel_shortcut"`
		PeriodShortcut  string `json:"period_shortcut"`
		NoChannels      string `json:"no_channels"`
		Anonymous       string `json:"anonymous"`
	} `json:"shell"`
}

// PitoCopy is the parsed snapshot, loaded once at init.
var PitoCopy = func() PitoCopySnapshot {
	var snap PitoCopySnapshot
	if err := json.Unmarshal(pitoCopyJSON, &snap); err != nil {
		panic("render: corrupt pito_copy.json: " + err.Error())
	}
	return snap
}()

var thinkingCopy, clipboardCopy = PitoCopy.Dictionaries, PitoCopy.Clipboard

// StartScreenTip picks a boot tip for the given seed from pito's own
// start-screen pool; "" when the pool is absent (older pito ref).
func StartScreenTip(seed int) string {
	tips := PitoCopy.StartScreen.Tips
	if len(tips) == 0 {
		return ""
	}
	if seed < 0 {
		seed = -seed
	}
	return tips[seed%len(tips)]
}

// ClipboardToastLine picks the copied-toast quip for the given seed from
// the owner's pito.copy.tui.clipboard pool (mirrored by tools/copygen).
// COPY LAW: an empty pool (older pito ref) returns "" — the toast's ✓
// glyph stands alone rather than the client inventing a line.
func ClipboardToastLine(seed int) string {
	if len(clipboardCopy) == 0 {
		return ""
	}
	if seed < 0 {
		seed = -seed
	}
	return clipboardCopy[seed%len(clipboardCopy)]
}

// thinkingFrames is the web's shared Braille spinner
// (Pito::Event::Concerns::BrailleFrames::FRAMES), cycled at 80ms a frame
// there (thinking_controller.js BRAILLE_INTERVAL) — so every SECOND 40ms
// animation tick here.
var thinkingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// thinkingGlyphs mirrors ThinkingComponent::GLYPHS — decorative resolved-
// state symbols, hardcoded in the web component too (not translatable
// copy), picked deterministically from the event id.
var thinkingGlyphs = []string{
	`\o/`, `¯\_(ツ)_/¯`, "(⌐■_■)", "o7", `\m/`, "^_^", "(•‿•)", ":3", ">_<", "( •_•)>⌐■-■",
}

// thinkingInterval is ThinkingComponent::INTERVAL_SECONDS — how long each
// cycled verb stays on screen, shared by server resolve and client cycle.
const thinkingInterval = 5 * time.Second

// NetworkShimmer is the web's --shimmer-network-* pair: accent-purple
// base with a brand-pito traveling band (the inverse of the action
// family). Worn by the thinking line and the @ai pending narration.
var NetworkShimmer = Gradient{Stops: []RGB{
	{0xbb, 0x9a, 0xf7}, // accent purple (base)
	{0x51, 0x70, 0xff}, // brand pito (band)
	{0xbb, 0x9a, 0xf7}, // back to base
}}

func (r *R) thinking(ev api.Event) string {
	var p struct {
		Dictionary     string   `json:"dictionary"`
		Order          []int    `json:"order"`
		StartedAt      string   `json:"started_at"`
		Resolved       bool     `json:"resolved"`
		ElapsedSeconds *float64 `json:"elapsed_seconds"`
		WordIndex      int      `json:"word_index"`
	}
	_ = json.Unmarshal(ev.Payload, &p)
	dict := thinkingCopy[p.Dictionary]

	if p.Resolved {
		// The component's resolved_glyph seed: event id, abs, mod pool.
		seed := ev.ID
		if seed < 0 {
			seed = -seed
		}
		glyph := thinkingGlyphs[seed%int64(len(thinkingGlyphs))]
		word := ""
		if p.WordIndex >= 0 && p.WordIndex < len(dict.Done) {
			word = dict.Done[p.WordIndex]
		} else if len(dict.Doing) > 0 {
			word = dict.Doing[0] // done_word's own fallback in the component
		}
		// COPY LAW: no word available (unknown dictionary) → the glyph and
		// the number stand alone; the locale's "%{word} for %{elapsed}s"
		// shape only renders with the server's own word in it.
		switch {
		case word != "" && p.ElapsedSeconds != nil:
			return r.dimStyle.Render(fmt.Sprintf("%s %s for %ss", glyph, word, formatElapsed(*p.ElapsedSeconds))) + "\n"
		case word != "":
			return r.dimStyle.Render(glyph+" "+word) + "\n"
		case p.ElapsedSeconds != nil:
			return r.dimStyle.Render(fmt.Sprintf("%s %ss", glyph, formatElapsed(*p.ElapsedSeconds))) + "\n"
		default:
			return r.dimStyle.Render(glyph) + "\n"
		}
	}

	// Pending: braille frame + the verb the shared time formula picks now.
	// COPY LAW: an unknown dictionary degrades to the frame and ellipsis
	// alone — the client never substitutes words of its own.
	frame := thinkingFrames[(r.ticks/2)%int64(len(thinkingFrames))]
	line := frame + " …"
	if idx := thinkingWordIndexAt(p.Order, r.elapsedSince(p.StartedAt)); idx >= 0 && idx < len(dict.Doing) {
		line = frame + " " + dict.Doing[idx] + "…"
	}
	if r.truecolor {
		return NetworkShimmer.Colorize(line, r.phase) + "\n"
	}
	return lipgloss.NewStyle().Foreground(ColorPrimary).Render(line) + "\n"
}

// pendingCanvas is the web's own pending metric canvas, glyph for glyph
// (pito-metric__plot while a fill job runs / before data lands): a row of
// ⠂ over a row of ⣀, 42 cells — the standard chart canvas width. COPY LAW
// (owner 2026-07-12): the TUI never invents words; where the web shows a
// drawing while work is in flight, the terminal shows the same drawing —
// the server's thinking event narrates, this canvas just holds the space.
func (r *R) pendingCanvas() string {
	rows := strings.Repeat("⠂", 42) + "\n" + strings.Repeat("⣀", 42)
	return r.dim(rows)
}

// thinkingWordIndexAt is ThinkingComponent.word_index_at: the order-th
// entry for the 5s step we're on. -1 when there's no order to walk.
func thinkingWordIndexAt(order []int, elapsed time.Duration) int {
	if len(order) == 0 {
		return -1
	}
	steps := int(elapsed / thinkingInterval)
	return order[steps%len(order)]
}

// elapsedSince parses a payload timestamp and measures against r.now(),
// clamped at zero (the component's elapsed_so_far).
func (r *R) elapsedSince(startedAt string) time.Duration {
	if startedAt == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return 0
	}
	if d := r.now().Sub(t); d > 0 {
		return d
	}
	return 0
}

// formatElapsed is the component's format_elapsed: ≤2 decimals, trailing
// fractional zeros stripped (0.224→"0.22", 0.5→"0.5", 1.0→"1").
func formatElapsed(seconds float64) string {
	s := fmt.Sprintf("%.2f", seconds)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}
