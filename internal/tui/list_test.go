package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func strList(items ...string) *list[string] {
	l := newList(func(s string) string { return s }, func(s string) string { return s })
	l.SetHeight(3)
	l.SetItems(items)
	return l
}

func window(l *list[string]) []string { items, _ := l.Window(); return items }

func TestListMoveAndScroll(t *testing.T) {
	l := strList("a", "b", "c", "d", "e", "f", "g")
	if !reflect.DeepEqual(window(l), []string{"a", "b", "c"}) || l.Cursor() != 0 {
		t.Fatalf("start: %v at %d", window(l), l.Cursor())
	}
	l.Move(1)
	l.Move(1)
	l.Move(1) // cursor leaves the window: it scrolls by one
	if !reflect.DeepEqual(window(l), []string{"b", "c", "d"}) || l.Cursor() != 3 {
		t.Errorf("after 3 moves: %v at %d", window(l), l.Cursor())
	}
	l.Bottom()
	if !reflect.DeepEqual(window(l), []string{"e", "f", "g"}) || l.Cursor() != 6 {
		t.Errorf("bottom: %v at %d", window(l), l.Cursor())
	}
	l.Move(10) // stops at the end
	if l.Cursor() != 6 {
		t.Errorf("past the end: %d", l.Cursor())
	}
	l.Top()
	if !reflect.DeepEqual(window(l), []string{"a", "b", "c"}) || l.Cursor() != 0 {
		t.Errorf("top: %v at %d", window(l), l.Cursor())
	}
	l.Move(-5)
	if l.Cursor() != 0 {
		t.Errorf("before the start: %d", l.Cursor())
	}
	l.Page(1) // a screenful less the line of context
	if l.Cursor() != 2 {
		t.Errorf("page down: %d", l.Cursor())
	}
	l.Page(-1)
	if l.Cursor() != 0 {
		t.Errorf("page up: %d", l.Cursor())
	}
}

func TestListFilter(t *testing.T) {
	l := strList("Alpha Game", "beta tool", "Gamma Game", "delta")
	l.SetFilter("game")
	if l.Len() != 2 || l.Total() != 4 || l.Filter() != "game" {
		t.Fatalf("len %d, total %d", l.Len(), l.Total())
	}
	if !reflect.DeepEqual(window(l), []string{"Alpha Game", "Gamma Game"}) {
		t.Errorf("window = %v", window(l))
	}
	l.Move(1)
	if got, _ := l.Selected(); got != "Gamma Game" {
		t.Errorf("selected %q", got)
	}
	// clearing the filter keeps the cursor on the same item
	l.SetFilter("")
	if got, _ := l.Selected(); got != "Gamma Game" || l.Cursor() != 2 {
		t.Errorf("after clearing: %q at %d", got, l.Cursor())
	}
	// a filter that matches nothing
	l.SetFilter("zzz")
	if _, ok := l.Selected(); ok || l.Len() != 0 || l.footer() != "" {
		t.Error("an empty list has no selection and no footer")
	}
	if len(window(l)) != 0 {
		t.Error("window of an empty list")
	}
	l.Move(1)
	l.Bottom()
	l.Top()
}

func TestListKeepsSelectionAcrossRefresh(t *testing.T) {
	l := strList("a", "b", "c", "d")
	l.SelectKey("c")
	l.SetItems([]string{"d", "c", "b", "a", "e"}) // reordered, one added
	if got, _ := l.Selected(); got != "c" {
		t.Errorf("selection moved to %q", got)
	}
	l.SetItems([]string{"x", "y"}) // the selected item is gone
	if got, ok := l.Selected(); !ok || (got != "x" && got != "y") {
		t.Errorf("cursor must stay valid, got %q %v", got, ok)
	}
	l.SetItems(nil)
	if _, ok := l.Selected(); ok {
		t.Error("nothing to select in an empty list")
	}
	if l.SelectKey("zz") {
		t.Error("SelectKey of a missing key")
	}
}

func TestListSelectedIsConsistentWithFilterAndRefresh(t *testing.T) {
	l := strList("one", "two", "three")
	l.SetFilter("t")
	l.SelectKey("three")
	l.SetItems([]string{"three", "ten", "one"}) // refresh while filtered
	if got, _ := l.Selected(); got != "three" {
		t.Errorf("got %q", got)
	}
	if !l.SelectKey("ten") {
		t.Error("ten passes the filter")
	}
	if l.SelectKey("one") {
		t.Error("one does not pass the filter, so it cannot be selected")
	}
}

func TestListSkipsSkippedRows(t *testing.T) {
	l := strList("a", "b", "c", "d", "e", "f").skipping(func(s string) bool { return s == " " })
	l.SetItems([]string{" ", "a", "b", " ", "c", "d", "e", " "})
	l.Top() // the leading spacer does not take the cursor
	if got, _ := l.Selected(); got != "a" {
		t.Errorf("top: %q", got)
	}
	l.Bottom() // the trailing spacer does not either
	if got, _ := l.Selected(); got != "e" {
		t.Errorf("bottom: %q", got)
	}
	l.Move(1)
	if got, _ := l.Selected(); got != "e" {
		t.Errorf("past the end: %q", got)
	}
	l.Top()
	l.Move(1)
	l.Move(1) // over the spacer between b and c
	if got, _ := l.Selected(); got != "c" {
		t.Errorf("two moves from the top: %q", got)
	}
	l.Move(-1) // back over the same spacer
	if got, _ := l.Selected(); got != "b" {
		t.Errorf("one move back: %q", got)
	}
	l.Bottom()
	if got, _ := l.Selected(); got != "e" {
		t.Fatalf("bottom: %q", got)
	}
	l.Scroll(1) // the window is now on d, e and the spacer
	if l.Click(2) {
		t.Error("clicking a spacer must do nothing")
	}
	if got, _ := l.Selected(); got != "e" {
		t.Errorf("a click on a spacer leaves the cursor alone: %q", got)
	}
	if !l.Click(0) {
		t.Error("clicking a real row still works")
	}
	if got, _ := l.Selected(); got != "d" {
		t.Errorf("clicked %q", got)
	}
	l.Top()
	l.Scroll(2) // the wheel drags the cursor onto the spacer: it moves on
	if got, _ := l.Selected(); got != "c" {
		t.Errorf("after a scroll: %q", got)
	}
	l.SetItems([]string{" "}) // nothing to rest on
	if _, ok := l.Selected(); ok {
		t.Error("a list of only spacers has no selection")
	}
}

func TestListSkipsRunsOfSkippedRows(t *testing.T) {
	sp := func(s string) bool { return s == " " }
	l := strList().skipping(sp)
	l.SetItems([]string{"a", " ", " "})
	l.Top()
	l.Move(1) // two spacers at the end: the cursor stays, it does not spin
	if got, _ := l.Selected(); got != "a" {
		t.Errorf("past trailing spacers: %q", got)
	}
	l.SetItems([]string{" ", " ", "a"})
	l.Bottom()
	l.Move(-1)
	if got, _ := l.Selected(); got != "a" {
		t.Errorf("past leading spacers: %q", got)
	}
	l.Page(-1)
	if got, _ := l.Selected(); got != "a" {
		t.Errorf("a page up over leading spacers: %q", got)
	}
	l.SetItems([]string{"a", " ", " "})
	l.cursor = 1
	l.rest() // forward finds nothing: it looks back
	if got, _ := l.Selected(); got != "a" {
		t.Errorf("rest: %q", got)
	}
}

func TestListFooterCountsOnlyRowsTheCursorRestsOn(t *testing.T) {
	l := strList().skipping(func(s string) bool { return s == " " })
	l.SetItems([]string{"a", " ", "b", " ", "c"})
	l.Top()
	if got := l.footer(); got != "1 of 3" {
		t.Errorf("top: %q", got)
	}
	l.Move(1)
	if got := l.footer(); got != "2 of 3" {
		t.Errorf("over a spacer: %q", got)
	}
	l.SetItems([]string{" "})
	if got := l.footer(); got != "" {
		t.Errorf("only spacers: %q", got)
	}
}

func TestListScrollKeepsTheCursorOnScreen(t *testing.T) {
	l := strList().skipping(func(s string) bool { return s == " " })
	l.SetItems([]string{"a", "b", " ", "c", "d", "e"})
	l.Bottom()
	l.Scroll(-3) // window a, b, spacer: dragged onto the spacer, the cursor goes to b, not off screen to c
	if got, _ := l.Selected(); got != "b" || !reflect.DeepEqual(window(l), []string{"a", "b", " "}) {
		t.Errorf("scroll up: %q in %v", got, window(l))
	}
}

func TestListClickAndScroll(t *testing.T) {
	l := strList("a", "b", "c", "d", "e", "f")
	if !l.Click(2) {
		t.Fatal("click inside")
	}
	if got, _ := l.Selected(); got != "c" {
		t.Errorf("clicked %q", got)
	}
	if l.Click(-1) || l.Click(9) {
		t.Error("clicks outside the list must be ignored")
	}
	l.Scroll(2) // the wheel moves the window, and the cursor with it if it would leave
	if !reflect.DeepEqual(window(l), []string{"c", "d", "e"}) {
		t.Errorf("window after scroll: %v", window(l))
	}
	if got, _ := l.Selected(); got != "c" {
		t.Errorf("cursor still on screen: %q", got)
	}
	l.Scroll(10)
	if !reflect.DeepEqual(window(l), []string{"d", "e", "f"}) {
		t.Errorf("scroll stops at the end: %v", window(l))
	}
	if got, _ := l.Selected(); got != "d" {
		t.Errorf("cursor dragged along: %q", got)
	}
	l.Scroll(-10)
	if !reflect.DeepEqual(window(l), []string{"a", "b", "c"}) {
		t.Errorf("scroll back: %v", window(l))
	}
	if l.Click(0) && l.Cursor() != 0 {
		t.Error("click row 0 after scrolling to the top")
	}
}

func TestListResize(t *testing.T) {
	l := strList("a", "b", "c", "d", "e")
	l.Bottom()
	l.SetHeight(5) // the window grows: everything fits again
	if !reflect.DeepEqual(window(l), []string{"a", "b", "c", "d", "e"}) {
		t.Errorf("after growing: %v", window(l))
	}
	l.SetHeight(2)
	if got, _ := l.Selected(); got != "e" {
		t.Errorf("selection lost: %q", got)
	}
	if !reflect.DeepEqual(window(l), []string{"d", "e"}) {
		t.Errorf("after shrinking: %v", window(l))
	}
	l.SetHeight(0) // nonsense heights are tolerated
}

func TestListScrollbarAndFooter(t *testing.T) {
	l := strList("a", "b", "c", "d", "e", "f")
	l.Move(1)
	if sb := l.scrollbar(); sb != (scrollbar{offset: 0, visible: 3, total: 6}) {
		t.Errorf("scrollbar = %+v", sb)
	}
	if l.footer() != "2 of 6" {
		t.Errorf("footer = %q", l.footer())
	}
}

func TestListRenderRows(t *testing.T) {
	th := testTheme(t)
	l := strList("a", "b", "c", "d")
	l.Move(1)
	var bases []lipgloss.Style
	rows := l.renderRows(10, true, th, func(item string, w int, base lipgloss.Style) string {
		bases = append(bases, base)
		return base.Render(fit(item, w))
	})
	if len(rows) != 3 {
		t.Fatalf("%d rows", len(rows))
	}
	for i, r := range rows {
		if width(r) != 10 {
			t.Errorf("row %d is %d wide", i, width(r))
		}
	}
	if rows[0] == rows[1] || !strings.Contains(rows[1], "\x1b[") {
		t.Error("the cursor row must be styled differently")
	}
	focused := rows[1]
	unfocused := l.renderRows(10, false, th, func(item string, w int, base lipgloss.Style) string {
		return base.Render(fit(item, w))
	})[1]
	if focused == unfocused {
		t.Error("the cursor row of an unfocused panel must look different")
	}
	if ansi.Strip(rows[1]) != fmt.Sprintf("%-10s", "b") {
		t.Errorf("row text = %q", ansi.Strip(rows[1]))
	}
}
