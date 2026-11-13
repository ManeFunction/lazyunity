package tui

import (
	"image/color"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestPaletteAnswersAreRecorded(t *testing.T) {
	th := &Theme{}
	for _, seq := range []string{
		"\x1b]4;12;rgb:5c5c/9d9d/ffff\x1b\\",
		"\x1b]4;2;rgb:a6/e3/a1\x07",
		"\x1b]4;1;rgb:ffff/0000/0000;3;rgb:ffff/ffff/0000\x1b\\",
	} {
		if !th.setPaletteFromOSC(seq) {
			t.Errorf("%q was not taken as a palette answer", seq)
		}
	}
	want := map[int]color.RGBA{
		12: {0x5c, 0x9d, 0xff, 0xff},
		2:  {0xa6, 0xe3, 0xa1, 0xff},
		1:  {0xff, 0x00, 0x00, 0xff},
		3:  {0xff, 0xff, 0x00, 0xff},
	}
	for i, w := range want {
		if got, ok := th.shownRGB(lipgloss.Color(strconv.Itoa(i))); !ok || !sameRGB(got, w) {
			t.Errorf("colour %d is shown as %v, want %v", i, got, w)
		}
	}
	// what the terminal did not report is not known
	if _, ok := th.shownRGB(lipgloss.Color("4")); ok {
		t.Error("unreported colour 4 is taken as known")
	}
	if _, ok := th.shownRGB(lipgloss.Color("#5c9dff")); !ok {
		t.Error("a hex colour is not taken as known")
	}
	for _, seq := range []string{"\x1b]11;rgb:0000/0000/0000\x07", "\x1b]4;99;rgb:ff/ff/ff\x07", "junk"} {
		if th.setPaletteFromOSC(seq) {
			t.Errorf("%q was taken as a palette answer", seq)
		}
	}
}

func TestScannerTrailFollowsTheTerminalPalette(t *testing.T) {
	m := newTestModel(t)
	m.th.ActiveBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	m.th.setPaletteFromOSC("\x1b]4;12;rgb:5c/9d/ff\x07")
	got := m.barTrail()[1].GetForeground()
	want := fadeTo(color.RGBA{0x5c, 0x9d, 0xff, 0xff}, color.Black, 0.65)
	if !sameRGB(got, want) {
		t.Errorf("first trail step is %v, want %v (faded from the terminal's brightblue)", got, want)
	}
}

// Warp reports its background but not its palette: the trail must not mix a
// guessed RGB for the head's colour, only shade the terminal's own colour.
func TestScannerTrailWithoutThePalette(t *testing.T) {
	m := newTestModel(t)
	m.th.ActiveBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	m.Update(tea.BackgroundColorMsg{Color: color.RGBA{0x19, 0x16, 0x22, 0xff}})
	trail := m.barTrail()
	if len(trail) != 3 {
		t.Fatalf("trail has %d steps, want 3 (bold, plain, faint)", len(trail))
	}
	for i, st := range trail {
		if st.GetForeground() != lipgloss.Color("12") {
			t.Errorf("step %d is %v, want the terminal's own colour 12", i, st.GetForeground())
		}
	}
	if !trail[0].GetBold() || trail[1].GetBold() || trail[1].GetFaint() || !trail[2].GetFaint() {
		t.Errorf("steps are not bold, plain, faint: %v %v %v", trail[0].GetBold(), trail[1].GetFaint(), trail[2].GetFaint())
	}
	// the scanner never goes dark, even holding at an end with so short a trail
	for i := 0; i < 80; i++ {
		m.spin = i
		if b := stripANSI(m.bar(&task{pct: -1, status: taskRunning}, 20)); !strings.Contains(b, barDone) {
			t.Errorf("frame %d has no lit cell: %q", i, b)
		}
	}
	// once the terminal does report the colour, the full gradient comes back
	m.th.setPaletteFromOSC("\x1b]4;12;rgb:5c/9d/ff\x07")
	if got := len(m.barTrail()); got != barTrail {
		t.Errorf("with the palette the trail has %d steps, want %d", got, barTrail)
	}
}

func TestScannerTrailFadesIntoTheBackground(t *testing.T) {
	m := newTestModel(t)
	m.th.ActiveBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("#5c9dff"))
	bg := color.RGBA{0x1e, 0x1e, 0x2e, 0xff}
	m.Update(tea.BackgroundColorMsg{Color: bg})
	trail := m.barTrail()
	// every step sits closer to the background than the one before
	dist := func(i int) int {
		r, g, b, _ := trail[i].GetForeground().RGBA()
		d := func(v uint32, base uint8) int { return abs(int(v>>8) - int(base)) }
		return d(r, bg.R) + d(g, bg.G) + d(b, bg.B)
	}
	for i := 2; i < len(trail); i++ {
		if dist(i) >= dist(i-1) {
			t.Errorf("trail step %d is not closer to the background than step %d", i, i-1)
		}
	}
	if last := dist(len(trail) - 1); last > dist(1)/5 {
		t.Errorf("the trail ends %d away from the background, want near it", last)
	}
	// a light background works the same way: the trail brightens into it
	if got := fadeTo(color.RGBA{0, 0, 0, 0xff}, color.White, 0); !sameRGB(got, color.White) {
		t.Errorf("fully faded is %v, want the background", got)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sameRGB(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar>>8 == br>>8 && ag>>8 == bg>>8 && ab>>8 == bb>>8
}
