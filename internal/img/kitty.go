// Package img shows images through the kitty graphics protocol where the
// terminal speaks it (kitty, ghostty, WezTerm) and stays silent elsewhere —
// capability parity with the web's thumbnails, detected dynamically, never
// required.
package img

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
)

// Supported reports whether the surrounding terminal is known to speak the
// kitty graphics protocol. Environment-based on purpose: querying the
// terminal after Bubble Tea owns stdin deadlocks (the glamour lesson).
func Supported() bool {
	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return true
	}
	if strings.Contains(os.Getenv("TERM"), "kitty") {
		return true
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "WezTerm", "ghostty":
		return true
	}
	return false
}

var nextID atomic.Uint32

// Shower pins one image at a time to a cell box on the terminal. Writes go
// straight to the TTY: one-shot atomic escapes, transmitted once per image
// (never per frame — the renderer's strings stay image-free).
type Shower struct {
	tty     io.Writer
	current uint32
}

func NewShower(tty io.Writer) *Shower {
	return &Shower{tty: tty}
}

// Show transmits PNG/JPEG bytes and places them in a cols×rows cell box
// whose top-left corner sits at (row, col), replacing any prior image.
func (s *Shower) Show(data []byte, row, col, cols, rows int) {
	if s == nil || s.tty == nil {
		return
	}
	id := nextID.Add(1)
	var b strings.Builder
	s.deleteInto(&b)

	// Transmit in 4KB base64 chunks: m=1 on all but the last (m=0).
	// f=100 = PNG/JPEG (kitty sniffs), q=2 = never reply (tea owns stdin).
	encoded := base64.StdEncoding.EncodeToString(data)
	first := true
	for len(encoded) > 0 {
		chunk := encoded
		if len(chunk) > 4096 {
			chunk = chunk[:4096]
		}
		encoded = encoded[len(chunk):]
		more := 0
		if len(encoded) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(&b, "\x1b_Ga=t,f=100,i=%d,q=2,m=%d;%s\x1b\\", id, more, chunk)
			first = false
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
	}

	// Save cursor, move to the box corner, place scaled-to-fit, restore.
	fmt.Fprintf(&b, "\x1b7\x1b[%d;%dH\x1b_Ga=p,i=%d,c=%d,r=%d,q=2\x1b\\\x1b8", row, col, id, cols, rows)

	_, _ = io.WriteString(s.tty, b.String())
	s.current = id
}

// Clear removes the currently shown image, if any.
func (s *Shower) Clear() {
	if s == nil || s.tty == nil || s.current == 0 {
		return
	}
	var b strings.Builder
	s.deleteInto(&b)
	_, _ = io.WriteString(s.tty, b.String())
}

func (s *Shower) deleteInto(b *strings.Builder) {
	if s.current != 0 {
		fmt.Fprintf(b, "\x1b_Ga=d,d=i,i=%d,q=2\x1b\\", s.current)
		s.current = 0
	}
}
