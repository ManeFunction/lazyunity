package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/ManeFunction/lazyunity/internal/config"
)

func testTheme(t *testing.T) *Theme {
	t.Helper()
	th, err := NewTheme(config.Default().GUI)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

// plain renders a frame and strips the colours.
func plain(th *Theme, f frame) []string {
	return strings.Split(ansi.Strip(th.renderFrame(f)), "\n")
}

func TestFrameLayout(t *testing.T) {
	th := testTheme(t)
	got := plain(th, frame{
		w: 24, h: 5, jump: "1", tabs: []string{"Projects"}, focused: true,
		lines: []string{"one", "two"}, footer: "1 of 3",
	})
	if len(got) != 5 {
		t.Fatalf("got %d lines, want 5", len(got))
	}
	for i, line := range got {
		if width(line) != 24 {
			t.Errorf("line %d is %d cells wide, want 24: %q", i, width(line), line)
		}
	}
	dashes := func(n int) string { return strings.Repeat("─", n) }
	want := []string{
		"╭─[1]─Projects" + dashes(9) + "╮",
		"│" + fit("one", 22) + "│",
		"│" + fit("two", 22) + "│",
		"│" + fit("", 22) + "│",
		"╰" + dashes(15) + "1 of 3─╯",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFrameTabsAndNoJump(t *testing.T) {
	th := testTheme(t)
	got := plain(th, frame{w: 40, h: 3, jump: "2", tabs: []string{"Installed", "Releases"}, tab: 1})
	if got[0] != "╭─[2]─Installed - Releases─────────────╮" {
		t.Errorf("tabs: %q", got[0])
	}
	got = plain(th, frame{w: 20, h: 3, tabs: []string{"Command log"}})
	if got[0] != "╭─Command log──────╮" {
		t.Errorf("no jump label: %q", got[0])
	}
}

func TestFrameTruncatesLongTitleAndContent(t *testing.T) {
	th := testTheme(t)
	got := plain(th, frame{
		w: 12, h: 3, jump: "1", tabs: []string{"A very long title"},
		lines: []string{"this line is much too long for the box"}, footer: "this footer is too long too",
	})
	for i, l := range got {
		if width(l) != 12 {
			t.Errorf("line %d is %d wide: %q", i, width(l), l)
		}
	}
	if !strings.Contains(got[0], "…") || !strings.Contains(got[1], "…") {
		t.Errorf("overflow should end in an ellipsis: %q", got[:2])
	}
	if strings.Contains(got[2], "footer") {
		t.Errorf("a footer that does not fit is dropped: %q", got[2])
	}
}

func TestFrameTooSmall(t *testing.T) {
	th := testTheme(t)
	for _, f := range []frame{{w: 2, h: 5}, {w: 10, h: 1}, {w: 0, h: 0}} {
		if got := th.renderFrame(f); got != "" {
			t.Errorf("%dx%d should draw nothing, got %q", f.w, f.h, got)
		}
	}
	// the smallest that works still has the right shape
	if got := plain(th, frame{w: 3, h: 2, tabs: []string{"abc"}}); len(got) != 2 || width(got[0]) != 3 {
		t.Errorf("3x2: %q", got)
	}
}

func TestFrameScrollbar(t *testing.T) {
	th := testTheme(t)
	rows := func(f frame) (thumb []int) {
		for i, l := range plain(th, f)[1 : f.h-1] {
			if strings.HasSuffix(l, "▐") {
				thumb = append(thumb, i)
			}
		}
		return
	}
	// everything fits: no thumb
	if got := rows(frame{w: 10, h: 7, scroll: scrollbar{offset: 0, visible: 5, total: 5}}); len(got) != 0 {
		t.Errorf("no overflow, thumb at %v", got)
	}
	// 5 rows showing 5 of 20, at the top: the thumb sits at the top and is 1 row tall
	if got := rows(frame{w: 10, h: 7, scroll: scrollbar{offset: 0, visible: 5, total: 20}}); len(got) != 1 || got[0] != 0 {
		t.Errorf("top: thumb at %v", got)
	}
	// scrolled to the end: the thumb is on the last row
	if got := rows(frame{w: 10, h: 7, scroll: scrollbar{offset: 15, visible: 5, total: 20}}); len(got) != 1 || got[0] != 4 {
		t.Errorf("bottom: thumb at %v", got)
	}
	// halfway
	if got := rows(frame{w: 10, h: 7, scroll: scrollbar{offset: 8, visible: 5, total: 20}}); len(got) != 1 || got[0] < 1 || got[0] > 3 {
		t.Errorf("middle: thumb at %v", got)
	}
	// a long list: a taller thumb
	if got := rows(frame{w: 10, h: 12, scroll: scrollbar{offset: 0, visible: 10, total: 20}}); len(got) != 5 {
		t.Errorf("half visible: thumb rows %v", got)
	}
}

func TestFrameBorderStyles(t *testing.T) {
	cfg := config.Default().GUI
	for name, corner := range map[string]string{"rounded": "╭", "single": "┌", "double": "╔", "bold": "┏"} {
		cfg.Border = name
		th, err := NewTheme(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got := plain(th, frame{w: 8, h: 3}); !strings.HasPrefix(got[0], corner) {
			t.Errorf("%s: top = %q", name, got[0])
		}
	}
	cfg.Border = "hidden"
	th, _ := NewTheme(cfg)
	got := plain(th, frame{w: 8, h: 3, tabs: []string{"x"}})
	if strings.ContainsAny(strings.Join(got, ""), "╭─│╮╰╯") {
		t.Errorf("hidden border drew something: %q", got)
	}
}

func TestFrameFocusChangesColour(t *testing.T) {
	th := testTheme(t)
	focused := th.renderFrame(frame{w: 10, h: 3, tabs: []string{"x"}, focused: true})
	plainFrame := th.renderFrame(frame{w: 10, h: 3, tabs: []string{"x"}})
	searching := th.renderFrame(frame{w: 10, h: 3, tabs: []string{"x"}, focused: true, searching: true})
	if focused == plainFrame {
		t.Error("a focused frame must look different from an unfocused one")
	}
	if searching == focused {
		t.Error("a frame being filtered must use the searching colour")
	}
	if !strings.Contains(focused, "\x1b[") {
		t.Error("a focused frame should be coloured")
	}
}

func TestFooterText(t *testing.T) {
	if footerText(0, 0) != "" || footerText(0, 1) != "1 of 1" || footerText(19, 20) != "20 of 20" {
		t.Error("footerText")
	}
}
