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
	// locked, in a multi menu, when not empty, is why the item's tick cannot
	// be changed now.
	locked string
	// heading rows label a group and cannot be picked.
	heading bool
	checked bool
	// installed, in a multi menu, marks what is already there: ticked, it
	// shows a green check; unticked, a red cross, as it is to be removed.
	installed bool
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
	// installs, in a multi menu, marks a ticked item that is not installed
	// yet with the pending arrow, as it is to be installed, in place of [x].
	installs bool
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
	// onToggle, in a multi menu, runs after an item is ticked (on) or
	// unticked, and may tick or untick others in items.
	onToggle func(items []menuItem, id string, on bool)
	// itemKeys are more keys of a multi menu, which act on the selected item
	// and leave the menu open.
	itemKeys map[string]func(m *Model, it menuItem) tea.Cmd
	// entry, when set, is a text field under the items of a multi menu.
	entry *menuEntry
	// pending, when set, describes the changes the menu would apply, "" when
	// there are none. Closing the menu with changes pending asks whether to
	// apply or discard them.
	pending func() string
	// confirmApply makes enter, with changes pending, ask before applying
	// them.
	confirmApply bool
}

// cancel closes the menu, or, with changes pending, asks what to do with them.
func (p *menuPopup) cancel() (func(*Model) tea.Cmd, bool) {
	if p.pending == nil {
		return nil, true
	}
	what := p.pending()
	if what == "" {
		return nil, true
	}
	return p.askApply("Unapplied changes", what, true), false
}

// askApply asks whether to apply the pending changes, listed in what; back
// returns to the menu, which stays open meanwhile. With discard, it offers to
// close the menu without applying them too.
func (p *menuPopup) askApply(title, what string, discard bool) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		c := &confirmPopup{
			title: title, yes: "Apply", no: "Back",
			message: what + "\n\nApply the changes?",
			onYes: func(m *Model) tea.Cmd {
				m.pop() // the menu
				return p.submit(m, p.picked())
			},
		}
		if discard {
			c.message = what + "\n\nApply the changes, or discard them?"
			c.extra = []confirmKey{{key: "d", label: "Discard", run: func(m *Model) tea.Cmd {
				m.pop() // the menu
				return nil
			}}}
		}
		m.push(c)
		return nil
	}
}

// menuEntry is a text field at the bottom of a multi menu. Down on the last
// item, or tab, moves into it; up or tab moves back to the items.
type menuEntry struct {
	label       string // before the field, such as "Add: "
	placeholder string // shown dimmed while the field is empty and not focused
	in          *input
	focused     bool
	// hints replace the menu's while the field is focused.
	hints []hintItem
	// busy, when not empty, says what the menu is waiting for. Until it is
	// cleared the menu takes no keys and the app does not quit.
	busy string
	// submit gets the text when enter is pressed in the field with text in it.
	// With the field empty, enter submits the menu as usual.
	submit func(m *Model, text string) tea.Cmd
}

// holding says what the menu waits for before it can be closed, "" when
// nothing.
func (p *menuPopup) holding() string {
	if p.entry == nil {
		return ""
	}
	return p.entry.busy
}

// focusEntry moves the cursor into the text field, or back to the last item.
func (p *menuPopup) focusEntry(on bool) {
	p.entry.focused = on
	if !on {
		p.list.Bottom()
	}
}

// updateEntry handles a message while the text field is focused.
func (p *menuPopup) updateEntry(m *Model, msg tea.Msg) (func(*Model) tea.Cmd, bool) {
	e := p.entry
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			if e.in.String() != "" {
				e.in.Set("")
				return nil, false
			}
			return p.cancel()
		case "enter":
			text := strings.TrimSpace(e.in.String())
			if text == "" || e.submit == nil {
				return p.activate(m)
			}
			return func(m *Model) tea.Cmd { return e.submit(m, text) }, false
		case "up", "ctrl+p", "tab", "shift+tab":
			p.focusEntry(false)
			return nil, false
		}
		e.in.Key(msg)
	case tea.PasteMsg:
		e.in.Paste(msg.Content)
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp {
			p.focusEntry(false)
		}
	}
	return nil, false
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
	).skipping(func(it menuItem) bool { return it.heading })
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
	return p
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

// toggle flips the tick of the selected item. It returns why it cannot,
// when the item is locked.
func (p *menuPopup) toggle() string {
	cur, ok := p.list.Selected()
	if !ok || cur.heading || cur.disabled != "" {
		return ""
	}
	if cur.locked != "" {
		return cur.locked
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
	if p.onToggle != nil {
		p.onToggle(items, cur.id, on)
	}
	p.list.SetItems(items)
	return ""
}

func (p *menuPopup) activate(m *Model) (func(*Model) tea.Cmd, bool) {
	if p.multi {
		picked := p.picked()
		if p.submit == nil {
			return nil, true
		}
		if p.confirmApply && p.pending != nil {
			if what := p.pending(); what != "" {
				return p.askApply("Apply changes", what, false), false
			}
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
	if e := p.entry; e != nil {
		switch {
		case e.busy != "":
			return nil, false
		case e.focused:
			return p.updateEntry(m, msg)
		}
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch msg.String() {
			case "tab":
				p.focusEntry(true)
				return nil, false
			case "down", "ctrl+n", "j":
				if p.list.AtBottom() {
					p.focusEntry(true)
					return nil, false
				}
			}
		case tea.PasteMsg:
			p.focusEntry(true)
			return p.updateEntry(m, msg)
		case tea.MouseWheelMsg:
			if msg.Button == tea.MouseWheelDown && p.list.AtBottom() {
				p.focusEntry(true)
				return nil, false
			}
		}
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.String()
		switch key {
		case "esc":
			if p.filterable && p.filter.String() != "" {
				p.filter.Set("")
				p.list.SetFilter("")
				return nil, false
			}
			return p.cancel()
		case "enter":
			return p.activate(m)
		case "up", "ctrl+p":
			p.list.Move(-1)
			return nil, false
		case "down", "ctrl+n":
			p.list.Move(1)
			return nil, false
		case "pgup":
			p.list.Page(-1)
			return nil, false
		case "pgdown":
			p.list.Page(1)
			return nil, false
		case "home":
			p.list.Top()
			return nil, false
		case "end":
			p.list.Bottom()
			return nil, false
		}
		if p.filterable {
			if p.filter.Key(msg) {
				p.list.SetFilter(p.filter.String())
			}
			return nil, false
		}
		switch key {
		case "j":
			p.list.Move(1)
			return nil, false
		case "k":
			p.list.Move(-1)
			return nil, false
		case "q":
			return p.cancel()
		case "space":
			if p.multi {
				if why := p.toggle(); why != "" {
					m.toastf(toastWarn, "%s", why)
				}
				return nil, false
			}
		}
		if fn := p.itemKeys[key]; fn != nil {
			if it, ok := p.list.Selected(); ok && !it.heading {
				return func(m *Model) tea.Cmd { return fn(m, it) }, false
			}
			return nil, false
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
			p.list.Move(-1)
		} else if msg.Button == tea.MouseWheelDown {
			p.list.Move(1)
		}
	case tea.PasteMsg:
		if p.filterable {
			p.filter.Paste(msg.Content)
			p.list.SetFilter(p.filter.String())
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
	if p.entry != nil {
		maxRows--
	}
	entryFocused := p.entry != nil && p.entry.focused
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
		lines = p.list.renderRows(inner, !entryFocused, th, func(it menuItem, wd int, base lipgloss.Style) string {
			if entryFocused {
				// the cursor is in the field: no row of the list looks selected
				base = lipgloss.NewStyle()
			}
			return p.drawItem(th, it, wd, base, keyW, rightW)
		})
	}

	box = th.renderFrame(frame{
		w: w, h: rows + 2 + b2i(p.filterable) + b2i(p.entry != nil), tabs: []string{p.title}, focused: true,
		lines: p.withEntryRow(m, p.withFilterRow(lines, rows, inner), inner), footer: p.list.footer(), scroll: p.list.scrollbar(),
	})
	if hints := p.hints; hints != nil {
		if entryFocused && p.entry.hints != nil {
			hints = p.entry.hints
		}
		tip = th.renderFrame(frame{w: w, h: 3, lines: []string{renderHints(th, hints, inner)}})
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
	return append(lines, label+p.filter.View(inner-width(label)))
}

// withEntryRow adds the text field's row, when the menu has one.
func (p *menuPopup) withEntryRow(m *Model, lines []string, inner int) []string {
	e := p.entry
	if e == nil {
		return lines
	}
	th := m.th
	label := th.ActiveBorder.Render(e.label)
	rest := inner - width(e.label)
	var field string
	switch {
	case e.busy != "":
		field = th.ActiveBorder.Render(m.spinnerFrame()) + " " + th.Dim.Render(truncate(e.busy, max(rest-2, 1)))
	case e.focused:
		field = e.in.View(max(rest, 2))
	case e.in.String() == "":
		field = th.Dim.Render(truncate(e.placeholder, max(rest, 1)))
	default:
		field = th.Dim.Render(truncate(e.in.String(), max(rest, 1)))
	}
	return append(lines, label+field)
}

func (p *menuPopup) drawItem(th *Theme, it menuItem, w int, base lipgloss.Style, keyW, rightW int) string {
	if it.heading {
		return th.ActiveBorder.Render(fit("─── "+it.label, w))
	}
	label := it.label
	mark := ""
	var markStyle lipgloss.Style
	switch {
	case p.multi && it.installed && it.checked:
		mark, markStyle = "[✓] ", th.Success.Inherit(base)
	case p.multi && it.installed:
		mark, markStyle = "[✗] ", th.Missing.Inherit(base)
	case p.multi && it.checked && p.installs:
		mark, markStyle = "[↓] ", th.Pending.Inherit(base)
	case p.multi && it.checked:
		label = "[x] " + label
	case p.multi:
		label = "[ ] " + label
	}
	labelStyle := base
	switch {
	case it.disabled != "":
		labelStyle = base.Strikethrough(true).Faint(true)
	case it.locked != "":
		labelStyle = base.Faint(true)
	}
	var sb strings.Builder
	used := 0
	if keyW > 0 {
		sb.WriteString(th.Key.Inherit(base).Render(padRight(it.key, keyW) + " "))
		used += keyW + 1
	}
	if mark != "" {
		sb.WriteString(markStyle.Render(mark))
		used += width(mark)
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
	no      string // label of the no answer, "Cancel" by default
	danger  bool
	onYes   func(m *Model) tea.Cmd
	onNo    func(m *Model) tea.Cmd
	// extra are more answers, each on a key of its own, shown between yes
	// and no.
	extra []confirmKey
}

// confirmKey is an extra answer of a confirmPopup.
type confirmKey struct {
	key, label string
	run        func(m *Model) tea.Cmd
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
	for _, x := range p.extra {
		if k.String() == x.key {
			return x.run, true
		}
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
	no := p.no
	if no == "" {
		no = "Cancel"
	}
	keys := th.OptionsText.Render("enter") + ": " + yes + "   "
	for _, x := range p.extra {
		keys += th.OptionsText.Render(x.key) + ": " + x.label + "   "
	}
	keys += th.OptionsText.Render("esc") + ": " + no
	body := make([]string, 0, len(lines)+2)
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
	var lines []string
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
	lines = append(lines, " "+p.in.ViewGhost(inner-2, ghost, th.Dim))
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

// copyChoices opens a Copy menu of label and value pairs; picking one puts its
// value on the clipboard.
func (m *Model) copyChoices(wanted int, choices [][2]string) {
	var items []menuItem
	for _, c := range choices {
		label, value := c[0], c[1]
		items = append(items, menuItem{label: label, right: truncate(value, wanted-16), run: func(m *Model) tea.Cmd {
			m.toastf(toastOK, "Copied the %s", strings.ToLower(label))
			return tea.SetClipboard(value)
		}})
	}
	menu := newMenu("Copy", items)
	menu.wanted = wanted
	m.push(menu)
}
