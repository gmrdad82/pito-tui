// chatScroller replaces bubbles/viewport for the conversation — the
// virtualized window (owner 2026-07-12, the launch-guarding perf issue:
// "as the conversation grows everything lags… In Web this is treated to
// render only visible things"). The old viewport was handed the ENTIRE
// joined transcript every refresh: SetContent split and re-measured
// every line of the whole scrollback, per keystroke and per animation
// frame — O(conversation) work 60 times a second. This scroller owns
// only the numbers (total, offset, height) and materializes just the
// visible window from the transcript's per-turn line caches
// (Transcript.WindowLines) plus the small tail (comet, notices).
//
// The method surface deliberately mirrors the bubbles viewport's so the
// ~26 call sites (scroll keys, wheel, glide, thumb geometry) swapped
// 1:1.
package ui

import "strings"

type chatScroller struct {
	height int
	width  int
	yoff   int
	total  int // transcript lines + tail lines, set by refreshViewport
}

func (s *chatScroller) SetHeight(h int) {
	if h < 1 {
		h = 1
	}
	s.height = h
	s.clamp()
}

func (s *chatScroller) SetWidth(w int) { s.width = w }

func (s *chatScroller) maxOffset() int {
	m := s.total - s.height
	if m < 0 {
		return 0
	}
	return m
}

func (s *chatScroller) clamp() {
	if s.yoff > s.maxOffset() {
		s.yoff = s.maxOffset()
	}
	if s.yoff < 0 {
		s.yoff = 0
	}
}

func (s *chatScroller) ScrollUp(n int)   { s.yoff -= n; s.clamp() }
func (s *chatScroller) ScrollDown(n int) { s.yoff += n; s.clamp() }
func (s *chatScroller) HalfPageUp()      { s.ScrollUp(s.height / 2) }
func (s *chatScroller) HalfPageDown()    { s.ScrollDown(s.height / 2) }
func (s *chatScroller) GotoTop()         { s.yoff = 0 }
func (s *chatScroller) GotoBottom()      { s.yoff = s.maxOffset() }
func (s *chatScroller) AtBottom() bool   { return s.yoff >= s.maxOffset() }
func (s *chatScroller) YOffset() int     { return s.yoff }
func (s *chatScroller) SetYOffset(n int) { s.yoff = n; s.clamp() }

func (s *chatScroller) TotalLineCount() int   { return s.total }
func (s *chatScroller) VisibleLineCount() int { return min(s.height, s.total) }

func (s *chatScroller) ScrollPercent() float64 {
	if s.maxOffset() == 0 {
		return 1
	}
	return float64(s.yoff) / float64(s.maxOffset())
}

// view materializes the visible window: transcript lines first, the tail
// (comet/notices — or the whole body when the transcript is empty)
// after, padded with blank lines to exactly height so the frame's layout
// math never moves.
func (m Model) scrollerView() string {
	s := m.sc
	lines := make([]string, 0, s.height)
	transcriptTotal := s.total - len(m.scTail)
	if transcriptTotal > 0 {
		lines = m.transcript.WindowLines(s.width, s.yoff, s.height)
	}
	// Tail lines that fall inside the window.
	tailStart := transcriptTotal // first tail line's global position
	for i, line := range m.scTail {
		pos := tailStart + i
		if pos >= s.yoff && pos < s.yoff+s.height && len(lines) < s.height {
			lines = append(lines, line)
		}
	}
	for len(lines) < s.height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
