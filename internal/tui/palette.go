package tui

import (
	"image/color"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The theme's named colours are the terminal's 16 ANSI colours, which every
// terminal paints its own way; their RGB in lipgloss is only xterm's guess
// (brightblue is #0000ff). Anything that does colour maths, like the scanner's
// fading trail, needs the colour the terminal really shows, so lazyunity asks
// for the palette at start (OSC 4) and keeps whatever the terminal answers.
// The background is asked for too (OSC 11), so the trail can fade into it.

// queryPaletteCmd asks the terminal for its 16 ANSI colours. Terminals that
// cannot answer stay silent, and the xterm values are used instead.
func queryPaletteCmd() tea.Cmd {
	var b strings.Builder
	for i := 0; i < 16; i++ {
		b.WriteString("\x1b]4;" + strconv.Itoa(i) + ";?\x07")
	}
	return tea.Raw(b.String())
}

// setPaletteFromOSC records a terminal's answer to a palette query, such as
// "\x1b]4;12;rgb:5c5c/9d9d/ffff\x1b\\". It reports whether seq was one.
func (th *Theme) setPaletteFromOSC(seq string) bool {
	data, ok := oscData(seq)
	if !ok || !strings.HasPrefix(data, "4;") {
		return false
	}
	// one answer may carry several index;colour pairs
	parts := strings.Split(strings.TrimPrefix(data, "4;"), ";")
	found := false
	for i := 0; i+1 < len(parts); i += 2 {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 || n >= len(th.palette) {
			continue
		}
		if c := ansi.XParseColor(parts[i+1]); c != nil {
			th.palette[n] = c
			found = true
		}
	}
	return found
}

// oscData is the body of an OSC sequence, without its introducer and terminator.
func oscData(seq string) (string, bool) {
	switch {
	case strings.HasPrefix(seq, "\x1b]"):
		seq = seq[2:]
	case strings.HasPrefix(seq, "\x9d"):
		seq = seq[1:]
	default:
		return "", false
	}
	for _, end := range []string{"\x1b\\", "\x07", "\x9c"} {
		if s, ok := strings.CutSuffix(seq, end); ok {
			return s, true
		}
	}
	return seq, true
}

// shownBackground is the terminal's background colour, or black when it did
// not say.
func (th *Theme) shownBackground() color.Color {
	if th.background != nil {
		return th.background
	}
	return color.Black
}

// shownColor is the colour the terminal paints c as: the palette it reported
// for an ANSI colour, or c itself.
func (th *Theme) shownColor(c color.Color) color.Color {
	if b, ok := c.(ansi.BasicColor); ok && int(b) < len(th.palette) && th.palette[b] != nil {
		return th.palette[b]
	}
	return c
}
