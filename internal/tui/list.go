package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// navigable is what the navigation keys need from whatever list has focus.
type navigable interface {
	Move(delta int)
	Page(delta int) // by a screenful
	Top()
	Bottom()
	Len() int
}

// list is a scrollable, filterable list of items with a cursor.
type list[T any] struct {
	items  []T
	vis    []int // indices into items that pass the filter
	cursor int   // index into vis
	offset int   // first visible row, index into vis
	height int   // rows on screen

	filter string
	text   func(T) string // what the filter matches against
	key    func(T) string // identity, to keep the cursor on an item across refreshes
	// skip, when set, marks rows such as spacers that the cursor does not
	// rest on.
	skip func(T) bool
}

func newList[T any](text, key func(T) string) *list[T] {
	return &list[T]{text: text, key: key, height: 1}
}

// skipping sets the rows the cursor does not rest on, such as spacers.
func (l *list[T]) skipping(skip func(T) bool) *list[T] {
	l.skip = skip
	l.clamp()
	return l
}

// SetItems replaces the items. The cursor stays on the same item when it is
// still there, and the filter is kept.
func (l *list[T]) SetItems(items []T) {
	var keep string
	var had bool
	if cur, ok := l.Selected(); ok && l.key != nil {
		keep, had = l.key(cur), true
	}
	l.items = items
	l.refilter()
	if had {
		l.SelectKey(keep)
	}
	l.clamp()
}

// Items returns every item, filtered or not.
func (l *list[T]) Items() []T { return l.items }

// SetFilter shows only items whose text contains s, ignoring case.
func (l *list[T]) SetFilter(s string) {
	var keep string
	var had bool
	if cur, ok := l.Selected(); ok && l.key != nil {
		keep, had = l.key(cur), true
	}
	l.filter = s
	l.refilter()
	if had {
		l.SelectKey(keep)
	}
	l.clamp()
}

// Filter is the active filter.
func (l *list[T]) Filter() string { return l.filter }

func (l *list[T]) refilter() {
	l.vis = l.vis[:0]
	needle := strings.ToLower(l.filter)
	for i, it := range l.items {
		if needle == "" || (l.text != nil && strings.Contains(strings.ToLower(l.text(it)), needle)) {
			l.vis = append(l.vis, i)
		}
	}
}

// Len is the number of items that pass the filter.
func (l *list[T]) Len() int { return len(l.vis) }

// Total is the number of items, ignoring the filter.
func (l *list[T]) Total() int { return len(l.items) }

// Cursor is the row the cursor is on, among the filtered items.
func (l *list[T]) Cursor() int { return l.cursor }

// Selected returns the item under the cursor.
func (l *list[T]) Selected() (T, bool) {
	if l.cursor < 0 || l.cursor >= len(l.vis) {
		var zero T
		return zero, false
	}
	return l.items[l.vis[l.cursor]], true
}

// SelectKey moves the cursor to the item with this key. It reports whether
// there is one (and it passes the filter).
func (l *list[T]) SelectKey(key string) bool {
	if l.key == nil {
		return false
	}
	for row, i := range l.vis {
		if l.key(l.items[i]) == key {
			l.cursor = row
			l.clamp()
			return true
		}
	}
	return false
}

// SetHeight tells the list how many rows are on screen.
func (l *list[T]) SetHeight(h int) {
	l.height = max(h, 1)
	l.clamp()
}

// Move moves the cursor, stopping at the ends. It steps over rows the cursor
// may not rest on.
func (l *list[T]) Move(delta int) {
	step := 1
	if delta < 0 {
		step = -1
	}
	l.cursor += delta
	for l.skipped(l.cursor) {
		if at := l.cursor + step; at >= 0 && at < len(l.vis) {
			l.cursor = at
		} else {
			l.cursor -= step // the end: back onto the nearest row it may rest on
		}
	}
	l.clamp()
}

// Page moves the cursor by a screenful.
func (l *list[T]) Page(delta int) { l.Move(delta * max(l.height-1, 1)) }

// Top moves the cursor to the first row it may rest on.
func (l *list[T]) Top() {
	l.cursor = 0
	for l.skipped(l.cursor) {
		l.cursor++
	}
	l.clamp()
}

// Bottom moves the cursor to the last row it may rest on.
func (l *list[T]) Bottom() {
	l.cursor = len(l.vis) - 1
	for l.skipped(l.cursor) {
		l.cursor--
	}
	l.clamp()
}

// Click moves the cursor to a row of the window (0 is the first row shown).
// Clicking a row the cursor may not rest on does nothing.
func (l *list[T]) Click(row int) bool {
	i := l.offset + row
	if row < 0 || i >= len(l.vis) || l.skipped(i) {
		return false
	}
	l.cursor = i
	l.clamp()
	return true
}

// Scroll moves the window by rows without moving the cursor off the screen.
func (l *list[T]) Scroll(delta int) {
	maxOff := max(len(l.vis)-l.height, 0)
	l.offset = clamp(l.offset+delta, 0, maxOff)
	l.cursor = clamp(l.cursor, l.offset, min(l.offset+l.height-1, len(l.vis)-1))
	l.rest()
}

// skipped reports whether the cursor may not rest on the visible row i.
func (l *list[T]) skipped(i int) bool {
	return i >= 0 && i < len(l.vis) && l.skip != nil && l.skip(l.items[l.vis[i]])
}

// rest moves the cursor onto the nearest row it may rest on, forward first.
// It runs after the visible rows change under the cursor, such as when a
// filter clears or a wheel scroll drags it along.
func (l *list[T]) rest() {
	for i := 0; i < len(l.vis) && l.skipped(l.cursor); i++ {
		if l.cursor+1 < len(l.vis) {
			l.cursor++
		} else {
			l.cursor--
		}
	}
}

// clamp keeps the cursor in range and the window around it.
func (l *list[T]) clamp() {
	n := len(l.vis)
	l.cursor = clamp(l.cursor, 0, max(n-1, 0))
	l.rest()
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+l.height {
		l.offset = l.cursor - l.height + 1
	}
	l.offset = clamp(l.offset, 0, max(n-l.height, 0))
}

// Window returns the items currently on screen and the row (among the filtered
// items) of the first one.
func (l *list[T]) Window() (items []T, first int) {
	end := min(l.offset+l.height, len(l.vis))
	for _, i := range l.vis[l.offset:end] {
		items = append(items, l.items[i])
	}
	return items, l.offset
}

// scrollbar returns the scroll position for the frame.
func (l *list[T]) scrollbar() scrollbar {
	return scrollbar{offset: l.offset, visible: l.height, total: len(l.vis)}
}

// footer is the "3 of 20" text.
func (l *list[T]) footer() string { return footerText(l.cursor, len(l.vis)) }

// renderRows draws the visible rows. draw receives the item, the width, and the
// base style for the row (the cursor style for the cursor row).
func (l *list[T]) renderRows(width int, focused bool, th *Theme, draw func(item T, width int, base lipgloss.Style) string) []string {
	items, first := l.Window()
	out := make([]string, 0, len(items))
	for i, it := range items {
		base := lipgloss.NewStyle()
		if first+i == l.cursor {
			base = th.SelectedInactive
			if focused {
				base = th.Selected
			}
		}
		out = append(out, draw(it, width, base))
	}
	return out
}

var _ navigable = (*list[int])(nil)
