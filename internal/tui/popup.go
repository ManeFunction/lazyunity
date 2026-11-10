package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// popup is a box drawn over the interface that takes the keyboard while it is open.
type popup interface {
	// update handles a message. next, if not nil, runs after the popup has been
	// closed (when done is true), so it may open another popup.
	update(m *Model, msg tea.Msg) (next func(*Model) tea.Cmd, done bool)
	// view draws the whole box, border included.
	view(m *Model) string
	// layers draws the box and, apart, what goes right under it (a menu
	// item's description). reserve is the height to keep free under the box
	// when it is placed, so that it does not move as below changes.
	layers(m *Model) (box, below string, reserve int)
	// click handles a click on a row of the box, 0 being the first row inside
	// the border. It answers as update does.
	click(m *Model, row int) (next func(*Model) tea.Cmd, done bool)
}

// wrapText wraps s to w cells, keeping its own line breaks.
func wrapText(s string, w int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(ansi.Wrap(para, w, " "), "\n")...)
	}
	return out
}

// boxWidth picks a popup width: wanted, but within the screen and at least min.
func (m *Model) boxWidth(wanted, minW int) int {
	return clamp(wanted, minW, max(m.w-4, minW))
}

// ---- menu ----------------------------------------------------------------

// menuItem is one row of a menu.
type menuItem struct {
	// id names the item: the menu selects and ticks items by it, and a multi
	// menu's submit tells what was ticked by it. newMenu gives an item without
	// one its label, made unique.
	id    string
	label string
	// desc explains the item in a box under the menu.
	desc string
	// key is a shortcut that picks the item.
	key string
	// right is dimmed text at the right edge, such as a size.
	right string
	// disabled, when not empty, is why the item cannot be picked.
	disabled string
	// heading rows label a group and cannot be picked.
	heading bool
	checked bool
	// group, in a multi menu, makes the items of the same group exclusive:
	// ticking one unticks the others.
	group string
	run   func(m *Model) tea.Cmd
}

// menuPopup is a list to choose from. In single mode Enter runs the item. In
// multi mode Space ticks items and Enter submits the ticked ones.
type menuPopup struct {
	title string
	list  *list[menuItem]
	multi bool
	// submit receives the ticked items in multi mode.
	submit func(m *Model, picked []menuItem) tea.Cmd
	// filterable menus filter as you type, like lazygit's keybindings menu.
	filterable bool
	filter     *input
	emptyMsg   string
	wanted     int // preferred inner width
	// hints, when set, are the menu's keys, shown in the box under the menu
	// in place of the items' descriptions.
	hints []hintItem
}

func newMenu(title string, items []menuItem) *menuPopup {
	p := &menuPopup{title: title, filter: newInput(""), wanted: 40}
	p.list = newList(
		func(it menuItem) string {
			if it.heading {
				return ""
			}
			return it.label + " " + it.key + " " + it.desc
		},
		func(it menuItem) string { return it.id },
	)
	items = append([]menuItem(nil), items...)
	seen := map[string]bool{}
	for i := range items {
		if items[i].id == "" {
			items[i].id = items[i].label
		}
		for base, n := items[i].id, 2; seen[items[i].id]; n++ {
			items[i].id = fmt.Sprintf("%s#%d", base, n)
		}
		seen[items[i].id] = true
	}
	// The real height is only known when the menu is drawn. Until then the window
	// is tall, so that moving off a first heading does not scroll it out of view.
	p.list.SetHeight(1 << 10)
	p.list.SetItems(items)
	p.skipHeadings(1)
	return p
}

// skipHeadings moves the cursor off a heading, in the given direction.
func (p *menuPopup) skipHeadings(dir int) {
	for i := 0; i < p.list.Len(); i++ {
		it, ok := p.list.Selected()
		if !ok || !it.heading {
			return
		}
		before := p.list.Cursor()
		p.list.Move(dir)
		if p.list.Cursor() == before { // hit an end: turn around
			dir = -dir
			p.list.Move(dir)
		}
	}
}

func (p *menuPopup) move(delta int) {
	dir := 1
	if delta < 0 {
		dir = -1
	}
	p.list.Move(delta)
	p.skipHeadings(dir)
}

func (p *menuPopup) picked() []menuItem {
	var out []menuItem
	for _, it := range p.list.Items() {
		if it.checked && !it.heading {
			out = append(out, it)
		}
	}
	return out
}

// toggle flips the tick of the selected item.
func (p *menuPopup) toggle() {
	cur, ok := p.list.Selected()
	if !ok || cur.heading || cur.disabled != "" {
		return
	}
	items := append([]menuItem(nil), p.list.Items()...)
	on := !cur.checked
	for i := range items {
		switch {
		case items[i].id == cur.id:
			items[i].checked = on
		case on && cur.group != "" && items[i].group == cur.group:
			items[i].checked = false
		}
	}
	p.list.SetItems(items)
}

func (p *menuPopup) activate(m *Model) (func(*Model) tea.Cmd, bool) {
	if p.multi {
		picked := p.picked()
		if p.submit == nil {
			return nil, true
		}
		return func(m *Model) tea.Cmd { return p.submit(m, picked) }, true
	}
	it, ok := p.list.Selected()
	if !ok || it.heading {
		return nil, false
	}
	if it.disabled != "" {
		m.toastf(toastWarn, "%s", it.disabled)
		return nil, false
	}
	return it.run, true
}

func (p *menuPopup) update(m *Model, msg tea.Msg) (func(*Model) tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.String()
		switch key {
		case "esc":
			if p.filterable && p.filter.String() != "" {
				p.filter.Set("")
				p.list.SetFilter("")
				p.skipHeadings(1)
				return nil, false
			}
			return nil, true
		case "enter":
			return p.activate(m)
		case "up", "ctrl+p":
			p.move(-1)
			return nil, false
		case "down", "ctrl+n":
			p.move(1)
			return nil, false
		case "pgup":
			p.list.Page(-1)
			p.skipHeadings(-1)
			return nil, false
		case "pgdown":
			p.list.Page(1)
			p.skipHeadings(1)
			return nil, false
		case "home":
			p.list.Top()
			p.skipHeadings(1)
			return nil, false
		case "end":
			p.list.Bottom()
			p.skipHeadings(-1)
			return nil, false
		}
		if p.filterable {
			if p.filter.Key(msg) {
				p.list.SetFilter(p.filter.String())
				p.skipHeadings(1)
			}
			return nil, false
		}
		switch key {
		case "j":
			p.move(1)
			return nil, false
		case "k":
			p.move(-1)
			return nil, false
		case "q":
			return nil, true
		case "space":
			if p.multi {
				p.toggle()
				return nil, false
			}
		}
		// an item's own shortcut
		if !p.multi {
			for _, it := range p.list.Items() {
				if it.key != "" && it.key == key && !it.heading {
					if it.disabled != "" {
						m.toastf(toastWarn, "%s", it.disabled)
						return nil, false
					}
					return it.run, true
				}
			}
		}
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp {
			p.move(-1)
		} else if msg.Button == tea.MouseWheelDown {
			p.move(1)
		}
	case tea.PasteMsg:
		if p.filterable {
			p.filter.Paste(msg.Content)
			p.list.SetFilter(p.filter.String())
			p.skipHeadings(1)
		}
	}
	return nil, false
}

// click picks the clicked row, as Enter would. A multi menu ignores clicks:
// a click there would mean ticking, which is not what Enter does.
func (p *menuPopup) click(m *Model, row int) (func(*Model) tea.Cmd, bool) {
	if p.multi || !p.list.Click(row) {
		return nil, false
	}
	return p.activate(m)
}

func (p *menuPopup) view(m *Model) string {
	box, tip, _ := p.layers(m)
	if tip != "" {
		box += "\n" + tip
	}
	return box
}

// maxTipLines is the most lines an item's description takes under a menu.
const maxTipLines = 4

// tipLines wraps a description for the box under the menu.
func tipLines(desc string, inner int) []string {
	dl := wrapText(desc, inner)
	if len(dl) > maxTipLines {
		dl = append(dl[:maxTipLines-1], truncate(dl[maxTipLines-1], inner))
	}
	return dl
}

// layers draws the menu and, apart, the description box of the selected item.
// reserve is the height of the tallest description box any item has: the
// menu is placed as if that box were always there, so that it stays put
// while the cursor moves between items with and without descriptions.
func (p *menuPopup) layers(m *Model) (box, tip string, reserve int) {
	th := m.th
	items := p.list.Items()
	// size the box to its content
	keyW, rightW, labelW := 0, 0, 0
	for _, it := range items {
		keyW = max(keyW, width(it.key))
		rightW = max(rightW, width(it.right))
		if !it.heading {
			labelW = max(labelW, width(it.label))
		}
	}
	if p.multi {
		labelW += 4 // "[x] "
	}
	inner := keyW + labelW + rightW + 2
	if keyW > 0 {
		inner++
	}
	if rightW > 0 {
		inner += 2
	}
	inner = max(inner, p.wanted, width(p.title)+4)
	w := m.boxWidth(inner+2, 30)
	inner = w - 2

	maxRows := max(m.h-8, 3)
	if p.filterable {
		maxRows--
	}
	rows := min(max(p.list.Len(), 1), maxRows)
	p.list.SetHeight(rows)

	var lines []string
	if p.list.Len() == 0 {
		msg := p.emptyMsg
		if msg == "" {
			msg = "Nothing to choose from"
		}
		lines = append(lines, th.Dim.Render(fit(msg, inner)))
	} else {
		lines = p.list.renderRows(inner, true, th, func(it menuItem, wd int, base lipgloss.Style) string {
			return p.drawItem(th, it, wd, base, keyW, rightW)
		})
	}

	box = th.renderFrame(frame{
		w: w, h: rows + 2 + b2i(p.filterable), tabs: []string{p.title}, focused: true,
		lines: p.withFilterRow(lines, rows, inner), footer: p.list.footer(), scroll: p.list.scrollbar(),
	})
	if p.hints != nil {
		tip = th.renderFrame(frame{w: w, h: 3, lines: []string{renderHints(th, p.hints, inner)}})
		return box, tip, 3
	}
	for _, it := range items {
		if it.desc != "" && !it.heading {
			reserve = max(reserve, len(tipLines(it.desc, inner))+2)
		}
	}
	// the selected item's description, in a box underneath
	if cur, ok := p.list.Selected(); ok && cur.desc != "" {
		dl := tipLines(cur.desc, inner)
		tip = th.renderFrame(frame{w: w, h: len(dl) + 2, lines: dl})
	}
	return box, tip, reserve
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// withFilterRow pads the item rows to the menu height and adds the filter row.
func (p *menuPopup) withFilterRow(lines []string, rows, inner int) []string {
	for len(lines) < rows {
		lines = append(lines, "")
	}
	if !p.filterable {
		return lines
	}
	label := "Filter: "
	return append(lines, label+p.filter.View(inner-width(label), true))
}

func (p *menuPopup) drawItem(th *Theme, it menuItem, w int, base lipgloss.Style, keyW, rightW int) string {
	if it.heading {
		return th.ActiveBorder.Render(fit("─── "+it.label, w))
	}
	label := it.label
	if p.multi {
		box := "[ ] "
		if it.checked {
			box = "[x] "
		}
		label = box + label
	}
	labelStyle := base
	if it.disabled != "" {
		labelStyle = base.Strikethrough(true).Faint(true)
	}
	var sb strings.Builder
	used := 0
	if keyW > 0 {
		sb.WriteString(th.Key.Inherit(base).Render(padRight(it.key, keyW) + " "))
		used += keyW + 1
	}
	avail := w - used
	if rightW > 0 {
		avail -= rightW + 2
	}
	sb.WriteString(labelStyle.Render(fit(label, max(avail, 1))))
	if rightW > 0 {
		sb.WriteString(base.Render("  "))
		sb.WriteString(th.Dim.Inherit(base).Render(padRight(it.right, rightW)))
	}
	return sb.String()
}

// choice is one option of a choiceMenu.
type choice struct{ id, label, desc string }

// choiceMenu offers one of several choices, with the current one marked and
// selected. Picking one calls onPick with its id.
func (m *Model) choiceMenu(title string, choices []choice, cur string, onPick func(m *Model, id string) tea.Cmd) {
	items := make([]menuItem, len(choices))
	at := 0
	for i, c := range choices {
		items[i] = menuItem{id: c.id, label: c.label, desc: c.desc, run: func(m *Model) tea.Cmd { return onPick(m, c.id) }}
		if c.id == cur {
			items[i].right, at = "current", i
		}
	}
	menu := newMenu(title, items)
	menu.wanted = 36
	// by position: newMenu names an item with an empty id after its label
	menu.list.SelectKey(menu.list.Items()[at].id)
	m.push(menu)
}

// ---- confirm -------------------------------------------------------------

// confirmPopup asks a yes-or-no question.
type confirmPopup struct {
	title   string
	message string
	yes     string // label of the yes answer, "Yes" by default
	danger  bool
	onYes   func(m *Model) tea.Cmd
	onNo    func(m *Model) tea.Cmd
}

func (p *confirmPopup) update(m *Model, msg tea.Msg) (func(*Model) tea.Cmd, bool) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil, false
	}
	switch k.String() {
	case "enter", "y":
		return p.onYes, true
	case "esc", "n", "q":
		return p.onNo, true
	}
	return nil, false
}

func (p *confirmPopup) layers(m *Model) (string, string, int)          { return p.view(m), "", 0 }
func (p *confirmPopup) click(*Model, int) (func(*Model) tea.Cmd, bool) { return nil, false }

func (p *confirmPopup) view(m *Model) string {
	th := m.th
	w := m.boxWidth(64, 36)
	lines := wrapText(p.message, w-4)
	yes := p.yes
	if yes == "" {
		yes = "Yes"
	}
	keys := th.OptionsText.Render("enter") + ": " + yes + "   " + th.OptionsText.Render("esc") + ": Cancel"
	body := make([]string, 0, len(lines)+3)
	body = append(body, "")
	for _, l := range lines {
		body = append(body, " "+th.Bold.Render(l))
	}
	body = append(body, "", " "+keys)
	t := th
	if p.danger {
		cp := *th
		cp.ActiveBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
		t = &cp
	}
	return t.renderFrame(frame{w: w, h: len(body) + 2, tabs: []string{p.title}, focused: true, lines: body})
}

// ---- prompt --------------------------------------------------------------

// promptPopup asks for a line of text.
type promptPopup struct {
	title    string
	label    string
	in       *input
	validate func(text string) string // returns what is wrong, or ""
	onSubmit func(m *Model, text string) tea.Cmd
	// complete returns completions for the text; Tab applies the first.
	complete func(text string) []string
	err      string
}

func (p *promptPopup) update(m *Model, msg tea.Msg) (func(*Model) tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return nil, true
		case "enter":
			text := strings.TrimSpace(p.in.String())
			if p.validate != nil {
				if e := p.validate(text); e != "" {
					p.err = e
					return nil, false
				}
			}
			return func(m *Model) tea.Cmd { return p.onSubmit(m, text) }, true
		case "tab":
			if p.complete != nil {
				if c := p.complete(p.in.String()); len(c) > 0 {
					p.in.Set(c[0])
				}
			}
			return nil, false
		case "right", "end":
			// at the end of the text, accept the ghost completion
			if p.complete != nil && p.in.cur == len(p.in.value) {
				if c := p.complete(p.in.String()); len(c) > 0 && len(c[0]) > len(p.in.String()) {
					p.in.Set(c[0])
					return nil, false
				}
			}
		}
		if p.in.Key(msg) {
			p.err = ""
		}
	case tea.PasteMsg:
		p.in.Paste(msg.Content)
		p.err = ""
	}
	return nil, false
}

func (p *promptPopup) layers(m *Model) (string, string, int)          { return p.view(m), "", 0 }
func (p *promptPopup) click(*Model, int) (func(*Model) tea.Cmd, bool) { return nil, false }

func (p *promptPopup) view(m *Model) string {
	th := m.th
	w := m.boxWidth(70, 40)
	inner := w - 2
	lines := []string{""}
	if p.label != "" {
		for _, l := range wrapText(p.label, inner-2) {
			lines = append(lines, " "+l)
		}
	}
	ghost := ""
	if p.complete != nil {
		text := p.in.String()
		if c := p.complete(text); len(c) > 0 && len(c[0]) > len(text) && strings.HasPrefix(c[0], text) {
			ghost = c[0][len(text):]
		}
	}
	lines = append(lines, " "+p.in.ViewGhost(inner-2, true, ghost, th.Dim))
	if p.err != "" {
		lines = append(lines, " "+th.Missing.Render(truncate(p.err, inner-2)))
	} else {
		lines = append(lines, "")
	}
	hint := "enter: Confirm   esc: Cancel"
	if p.complete != nil {
		hint = "enter: Confirm   tab/→: Complete   esc: Cancel"
	}
	lines = append(lines, " "+th.OptionsText.Render(hint))
	return th.renderFrame(frame{w: w, h: len(lines) + 2, tabs: []string{p.title}, focused: true, lines: lines})
}

// ---- info ----------------------------------------------------------------

// infoPopup shows scrollable text. Long lines are wrapped, never cut: what it
// shows is often a path or the output of a command.
type infoPopup struct {
	title  string
	lines  []string
	danger bool // the text is an error: drawn in red
	offset int
	height int

	wrapped []string // lines wrapped to the width of the last drawing
	wrapW   int
}

// rows is the text as drawn, wrapped to w cells.
func (p *infoPopup) rows(w int) []string {
	if p.wrapped != nil && p.wrapW == w {
		return p.wrapped
	}
	p.wrapW = w
	p.wrapped = make([]string, 0, len(p.lines))
	for _, l := range p.lines {
		if width(l) <= w {
			p.wrapped = append(p.wrapped, l)
			continue
		}
		p.wrapped = append(p.wrapped, strings.Split(ansi.Wrap(l, w, " /\\-"), "\n")...)
	}
	return p.wrapped
}

func (p *infoPopup) total() int {
	if p.wrapped != nil {
		return len(p.wrapped)
	}
	return len(p.lines)
}

func (p *infoPopup) update(m *Model, msg tea.Msg) (func(*Model) tea.Cmd, bool) {
	maxOff := max(p.total()-p.height, 0)
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q", "enter":
			return nil, true
		case "j", "down":
			p.offset++
		case "k", "up":
			p.offset--
		case "pgdown", "ctrl+d", "space":
			p.offset += max(p.height-1, 1)
		case "pgup", "ctrl+u":
			p.offset -= max(p.height-1, 1)
		case "home", "<":
			p.offset = 0
		case "end", ">":
			p.offset = maxOff
		}
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp {
			p.offset -= 3
		} else if msg.Button == tea.MouseWheelDown {
			p.offset += 3
		}
	}
	p.offset = clamp(p.offset, 0, maxOff)
	return nil, false
}

func (p *infoPopup) layers(m *Model) (string, string, int)          { return p.view(m), "", 0 }
func (p *infoPopup) click(*Model, int) (func(*Model) tea.Cmd, bool) { return nil, false }

func (p *infoPopup) view(m *Model) string {
	w := m.boxWidth(80, 40)
	inner := w - 2
	rows := p.rows(inner - 2)
	p.height = clamp(len(rows), 1, max(m.h-6, 3))
	p.offset = clamp(p.offset, 0, max(len(rows)-p.height, 0))
	end := min(p.offset+p.height, len(rows))
	shown := make([]string, 0, p.height)
	for _, l := range rows[p.offset:end] {
		if p.danger {
			l = m.th.Missing.Render(l)
		}
		shown = append(shown, " "+l)
	}
	footer := ""
	if len(rows) > p.height {
		footer = fmt.Sprintf("%d of %d", min(p.offset+p.height, len(rows)), len(rows))
	}
	return m.th.renderFrame(frame{
		w: w, h: p.height + 2, tabs: []string{p.title}, focused: true, lines: shown, footer: footer,
		scroll: scrollbar{offset: p.offset, visible: p.height, total: len(rows)},
	})
}
