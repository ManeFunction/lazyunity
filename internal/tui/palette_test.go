package tui

import (
	"image/color"
	"strconv"
	"testing"

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
		if got := th.shownColor(lipgloss.Color(strconv.Itoa(i))); !sameRGB(got, w) {
			t.Errorf("colour %d is shown as %v, want %v", i, got, w)
		}
	}
	// what the terminal did not report keeps xterm's value
	if got := th.shownColor(lipgloss.Color("4")); !sameRGB(got, color.RGBA{0, 0, 0x80, 0xff}) {
		t.Errorf("unreported colour 4 is %v", got)
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
	want := fadeToBlack(color.RGBA{0x5c, 0x9d, 0xff, 0xff}, 0.65)
	if !sameRGB(got, want) {
		t.Errorf("first trail step is %v, want %v (faded from the terminal's brightblue)", got, want)
	}
}

func sameRGB(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar>>8 == br>>8 && ag>>8 == bg>>8 && ab>>8 == bb>>8
}
