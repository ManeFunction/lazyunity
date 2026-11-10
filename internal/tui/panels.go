package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// tab is one list shown inside a panel.
type tab interface {
	title() string
	ctx() ctxID
	nav() navigable
	// rows draws the visible rows, each w cells wide.
	rows(m *Model, w int, focused bool) []string
	footer() string
	scrollbar() scrollbar
	setHeight(h int)
	setFilter(s string)
	filter() string
	click(row int) bool
	scroll(delta int)
	// emptyMsg is shown when there is nothing to list.
	emptyMsg() string
	// filterRow reports whether the tab keeps its filter on its own last row,
	// as the keybindings menu does, rather than on the bottom line.
	filterRow() bool

	// A tab that loads its items lazily does it from these: shown runs when
	// the tab comes into view, moved after the cursor moved, filtered after
	// the filter changed.
	shown(m *Model) tea.Cmd
	moved(m *Model) tea.Cmd
	filtered(m *Model) tea.Cmd
	// loading reports that more items are on their way.
	loading() bool
	// status is shown at the right end of the filter row.
	status() string
}

// listTab is a tab backed by a list of T.
type listTab[T any] struct {
	*list[T]
	name    string
	context ctxID
	empty   string
	// filterLine keeps a filter row at the bottom of the tab.
	filterLine bool
	// prep runs before the rows are drawn, to measure columns.
	prep func(m *Model, w int)
	draw func(m *Model, it T, w int, base lipgloss.Style) string
}

func (t *listTab[T]) title() string        { return t.name }
func (t *listTab[T]) ctx() ctxID           { return t.context }
func (t *listTab[T]) nav() navigable       { return t.list }
func (t *listTab[T]) footer() string       { return t.list.footer() }
func (t *listTab[T]) scrollbar() scrollbar { return t.list.scrollbar() }
func (t *listTab[T]) setHeight(h int)      { t.list.SetHeight(h - b2i(t.filterLine)) }
func (t *listTab[T]) filterRow() bool      { return t.filterLine }
func (t *listTab[T]) setFilter(s string)   { t.list.SetFilter(s) }
func (t *listTab[T]) filter() string       { return t.list.Filter() }
func (t *listTab[T]) click(row int) bool   { return t.list.Click(row) }
func (t *listTab[T]) scroll(d int)         { t.list.Scroll(d) }
func (t *listTab[T]) emptyMsg() string     { return t.empty }

// A plain list has all of its items from the start.
func (t *listTab[T]) shown(*Model) tea.Cmd    { return nil }
func (t *listTab[T]) moved(*Model) tea.Cmd    { return nil }
func (t *listTab[T]) filtered(*Model) tea.Cmd { return nil }
func (t *listTab[T]) loading() bool           { return false }
func (t *listTab[T]) status() string          { return "" }

func (t *listTab[T]) rows(m *Model, w int, focused bool) []string {
	if t.prep != nil {
		t.prep(m, w)
	}
	return t.list.renderRows(w, focused, m.th, func(it T, w int, base lipgloss.Style) string {
		return t.draw(m, it, w, base)
	})
}

// drill is a list that temporarily replaces a panel's tabs: the modules of one
// Editor. Esc goes back.
type drill struct {
	title string
	tab   tab
	// editor is the Editor whose modules the drill shows.
	editor editorRow
}

// tabsLoading reports whether any tab is loading.
func (m *Model) tabsLoading() bool {
	for _, p := range m.panels {
		for _, t := range p.tabs {
			if t.loading() {
				return true
			}
		}
	}
	return false
}

// panel is a frame with one or more tabs.
type panel struct {
	jump  string
	tabs  []tab
	cur   int
	drill *drill
}

func (p *panel) active() tab {
	if p.drill != nil {
		return p.drill.tab
	}
	return p.tabs[p.cur]
}

// titles is what the frame shows in its top border.
func (p *panel) titles() ([]string, int) {
	if p.drill != nil {
		return []string{p.drill.title}, 0
	}
	names := make([]string, len(p.tabs))
	for i, t := range p.tabs {
		names[i] = t.title()
	}
	return names, p.cur
}

func (p *panel) setHeight(h int) {
	for _, t := range p.tabs {
		t.setHeight(h)
	}
	if p.drill != nil {
		p.drill.tab.setHeight(h)
	}
}

// switchTab moves to another tab; it does nothing while drilled in.
func (p *panel) switchTab(delta int) bool {
	if p.drill != nil || len(p.tabs) < 2 {
		return false
	}
	p.cur = (p.cur + delta + len(p.tabs)) % len(p.tabs)
	return true
}

// tabAtX finds the tab whose name is drawn at column x of the top border, or -1.
func (p *panel) tabAtX(x int, showJump bool) int {
	if p.drill != nil {
		return -1
	}
	col := 2 // corner and one dash
	if showJump {
		col += width("["+p.jump+"]") + 1
	}
	for i, t := range p.tabs {
		if i > 0 {
			col += 3 // " - "
		}
		w := width(t.title())
		if x >= col && x < col+w {
			return i
		}
		col += w
	}
	return -1
}

// ---- row drawing ----------------------------------------------------------

// rowBuilder assembles one list row out of styled segments. Every segment
// inherits the row's base style, so a cursor row keeps its background all the
// way across.
type rowBuilder struct {
	base lipgloss.Style
	sb   strings.Builder
	used int
}

func newRow(base lipgloss.Style) *rowBuilder { return &rowBuilder{base: base} }

// add writes text in a cell of the given width: cut with … or padded.
func (r *rowBuilder) add(text string, st lipgloss.Style, cells int) *rowBuilder {
	if cells <= 0 {
		return r
	}
	r.sb.WriteString(st.Inherit(r.base).Render(fit(text, cells)))
	r.used += cells
	return r
}

// gap writes n spaces.
func (r *rowBuilder) gap(n int) *rowBuilder {
	if n > 0 {
		r.sb.WriteString(r.base.Render(strings.Repeat(" ", n)))
		r.used += n
	}
	return r
}

// text writes text as it is, without padding.
func (r *rowBuilder) text(text string, st lipgloss.Style) *rowBuilder {
	r.sb.WriteString(st.Inherit(r.base).Render(text))
	r.used += width(text)
	return r
}

// done pads the row to w cells and returns it.
func (r *rowBuilder) done(w int) string {
	if r.used < w {
		r.sb.WriteString(r.base.Render(strings.Repeat(" ", w-r.used)))
	}
	return r.sb.String()
}

// maxWidth is the widest of the strings.
func maxWidth(strs []string, limit int) int {
	w := 0
	for _, s := range strs {
		w = max(w, width(s))
	}
	return min(w, limit)
}
