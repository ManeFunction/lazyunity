package tui

import (
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// Progress bars are black squares (U+25A0) for what is done and middle dots
// (U+00B7) for the rest, so they read without color: [■■■■······].
// The brackets count toward the width.
const (
	barDone = "■"
	barTodo = "·"

	// barTrail is how many cells the scanner's bright head and fading trail
	// can take. barHold is how many frames it rests at each end while the
	// trail fades before it turns back.
	barTrail = 7
	barHold  = 3
)

// barCells is how many of w cells are done at pct percent.
func barCells(pct float64, w int) int {
	return int(math.Round(clampF(pct, 0, 100) / 100 * float64(w)))
}

// bar draws a task's progress bar, w cells wide with its brackets. When the
// progress cannot be told, a scanner sweeps back and forth instead: a bright
// head with a trail that fades into the terminal background, like opencode's progress bar.
func (m *Model) bar(t *task, w int) string {
	on, off := m.th.ActiveBorder, m.th.Dim
	in := max(w-2, 1)
	var body string
	if t.indeterminate() && t.status != taskDone {
		body = m.scannerBar(in, off)
	} else {
		pct := t.pct
		if t.status == taskDone {
			pct = 100
		}
		n := barCells(pct, in)
		body = barRun(on, barDone, n) + barRun(off, barTodo, in-n)
	}
	return m.th.BarEdge.Render("[") + body + m.th.BarEdge.Render("]")
}

// scannerState places the scanner's head for a frame: it sweeps right, holds
// while the trail fades, sweeps left, and holds again. fade is how many frames
// of the hold have passed; it darkens the head and trail one step at a time.
func scannerState(frame, w int) (pos, dir, fade int) {
	if w <= 1 {
		return 0, 1, 0
	}
	cycle := w + barHold + (w - 1) + barHold
	f := frame % cycle
	switch {
	case f < w:
		return f, 1, 0
	case f < w+barHold:
		return w - 1, 1, f - w + 1
	case f < cycle-barHold:
		return w - 2 - (f - w - barHold), -1, 0
	default:
		return 0, -1, f - (cycle - barHold) + 1
	}
}

// scannerDistance is how many cells i trails the head, or -1 when i is ahead
// of it (where a trail cannot be).
func scannerDistance(i, pos, dir int) int {
	if dir > 0 {
		return pos - i
	}
	return i - pos
}

// scannerBar draws the indeterminate bar: the head lit in the active colour,
// then a run of squares behind it fading into the background.
func (m *Model) scannerBar(w int, off lipgloss.Style) string {
	pos, dir, fade := scannerState(m.spin, w)
	trail := m.barTrail()
	// a short trail must not fade out entirely while it holds at an end
	fade = min(fade, len(trail)-1)
	var b strings.Builder
	for i := 0; i < w; {
		d := scannerDistance(i, pos, dir)
		if d >= 0 && d+fade < len(trail) {
			b.WriteString(trail[d+fade].Render(barDone))
			i++
			continue
		}
		j := i + 1
		for j < w {
			if d := scannerDistance(j, pos, dir); d >= 0 && d+fade < len(trail) {
				break
			}
			j++
		}
		b.WriteString(barRun(off, barTodo, j-i))
		i = j
	}
	return b.String()
}

// barTrail is the scanner's palette: the head in the active colour, then the
// steps behind it. Each step keeps 65% of the one before it, so no two cells
// share a colour and the trail melts smoothly into the terminal's background.
//
// That takes the RGB the terminal really paints the head with. When the head
// is an ANSI colour the terminal would not report (Warp does not answer the
// palette query), the trail would start from a guessed RGB and show a
// different hue behind the head, so the head is drawn in that same RGB too:
// the whole scanner is then one gradient, though its blue may differ a little
// from the terminal's own.
func (m *Model) barTrail() []lipgloss.Style {
	head := m.th.ActiveBorder
	c := head.GetForeground()
	if _, none := c.(lipgloss.NoColor); none {
		c = m.th.BarEdge.GetForeground()
		head = head.Foreground(c).Bold(true)
	}
	from, shown := m.th.paintRGB(c)
	if !shown {
		head = head.Foreground(from)
	}
	to := m.th.shownBackground()
	trail := make([]lipgloss.Style, barTrail)
	trail[0] = head
	for i := 1; i < barTrail; i++ {
		trail[i] = lipgloss.NewStyle().Foreground(fadeTo(from, to, math.Pow(0.65, float64(i))))
	}
	return trail
}

// fadeTo mixes colour c into bg; 1 is c, 0 is bg.
func fadeTo(c, bg color.Color, f float64) color.RGBA {
	r, g, b, _ := c.RGBA()
	br, bgg, bb, _ := bg.RGBA()
	cell := func(v, base uint32) uint8 {
		mixed := float64(base)/257 + (float64(v)-float64(base))/257*f
		return uint8(clampF(math.Round(mixed), 0, 255))
	}
	return color.RGBA{R: cell(r, br), G: cell(g, bgg), B: cell(b, bb), A: 0xff}
}

// barRun is n cells of one bar character in a style.
func barRun(st lipgloss.Style, ch string, n int) string {
	if n <= 0 {
		return ""
	}
	return st.Render(strings.Repeat(ch, n))
}
