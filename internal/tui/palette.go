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
// Some terminals answer only the background (Warp), so the two are used apart.

// queryPaletteCmd asks the terminal for its 16 ANSI colours. Terminals that
// cannot answer stay silent, and those colours stay unknown.
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

// shownRGB is the colour the terminal paints c as, and whether that is known:
// an ANSI colour is only known once the terminal has reported its palette
// entry, since every terminal paints those its own way.
func (th *Theme) shownRGB(c color.Color) (color.Color, bool) {
	if b, ok := c.(ansi.BasicColor); ok {
		if int(b) < len(th.palette) && th.palette[b] != nil {
			return th.palette[b], true
		}
		return c, false
	}
	return c, true
}

// fallbackPalette is what an ANSI colour is drawn as when the terminal keeps
// its palette to itself (Warp answers OSC 11 but not OSC 4). It is a typical
// modern dark palette rather than xterm's, whose brightblue (#0000ff) is far
// from what any current terminal shows; brightblue is lazyunity's own blue.
var fallbackPalette = [16]color.RGBA{
	{0x00, 0x00, 0x00, 0xff}, {0xcd, 0x31, 0x31, 0xff}, {0x0d, 0xbc, 0x79, 0xff}, {0xe5, 0xe5, 0x10, 0xff},
	{0x24, 0x72, 0xc8, 0xff}, {0xbc, 0x3f, 0xbc, 0xff}, {0x11, 0xa8, 0xcd, 0xff}, {0xe5, 0xe5, 0xe5, 0xff},
	{0x66, 0x66, 0x66, 0xff}, {0xf1, 0x4c, 0x4c, 0xff}, {0x23, 0xd1, 0x8b, 0xff}, {0xf5, 0xf5, 0x43, 0xff},
	{0x5c, 0x9d, 0xff, 0xff}, {0xd6, 0x70, 0xd6, 0xff}, {0x29, 0xb8, 0xdb, 0xff}, {0xff, 0xff, 0xff, 0xff},
}

// paintRGB is an RGB to draw c with when doing colour maths: the terminal's
// own when it is known, lazyunity's fallback for an unreported ANSI colour.
// It reports whether that is what the terminal itself shows for c.
func (th *Theme) paintRGB(c color.Color) (color.Color, bool) {
	if rgb, ok := th.shownRGB(c); ok {
		return rgb, true
	}
	if b, ok := c.(ansi.BasicColor); ok && int(b) < len(fallbackPalette) {
		return fallbackPalette[b], false
	}
	return c, false
}
