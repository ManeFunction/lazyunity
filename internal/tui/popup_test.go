package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// ran records which menu items were run.
type ran struct{ got []string }

func (r *ran) item(label string) menuItem {
	return menuItem{label: label, run: func(*Model) tea.Cmd { r.got = append(r.got, label); return nil }}
}

func TestMenuNavigationSkipsHeadings(t *testing.T) {
	m := newTestModel(t)
	var r ran
	menu := newMenu("Test", []menuItem{
		{label: "First group", heading: true}, r.item("a"), r.item("b"),
		{label: "Second group", heading: true}, r.item("c"),
	})
	m.push(menu)
	cur := func() string { it, _ := menu.list.Selected(); return it.label }
	if cur() != "a" {
		t.Fatalf("the cursor must start on the first real item, got %q", cur())
	}
	press(m, "j")
	press(m, "j") // steps over the heading
	if cur() != "c" {
		t.Errorf("down over a heading: %q", cur())
	}
	press(m, "j") // already at the end
	if cur() != "c" {
		t.Errorf("at the end: %q", cur())
	}
	press(m, "k", "k")
	if cur() != "a" {
		t.Errorf("up over a heading: %q", cur())
	}
	press(m, "enter")
	if len(m.popups) != 0 || strings.Join(r.got, ",") != "a" {
		t.Errorf("enter should run the item and close: popups=%d ran=%v", len(m.popups), r.got)
	}
}

func TestMenuEndsAndPaging(t *testing.T) {
	m := newTestModel(t)
	var items []menuItem
	for _, l := range []string{"a", "b", "c", "d", "e", "f"} {
		items = append(items, menuItem{label: l})
	}
	menu := newMenu("Test", items)
	m.push(menu)
	cur := func() string { it, _ := menu.list.Selected(); return it.label }
	press(m, "end")
	if cur() != "f" {
		t.Errorf("end: %q", cur())
	}
	press(m, "home")
	if cur() != "a" {
		t.Errorf("home: %q", cur())
	}
	press(m, "pgdown")
	if cur() == "a" {
		t.Error("pgdown did not move")
	}
	press(m, "pgup")
	press(m, "down", "up", "ctrl+n", "ctrl+p")
}

func TestMenuShortcutsAndDisabledItems(t *testing.T) {
	m := newTestModel(t)
	var r ran
	a := r.item("alpha")
	a.key = "a"
	b := r.item("beta")
	b.key = "b"
	b.disabled = "Beta is not available"
	menu := newMenu("Test", []menuItem{a, b})
	m.push(menu)

	press(m, "b") // a disabled item cannot be picked, and says why
	if len(m.popups) != 1 || len(r.got) != 0 {
		t.Fatalf("a disabled shortcut ran: popups=%d ran=%v", len(m.popups), r.got)
	}
	if !strings.Contains(screenText(m), "Beta is not available") {
		t.Error("the reason should be shown")
	}
	press(m, "j", "enter") // cursor on the disabled item, enter does nothing either
	if len(m.popups) != 1 || len(r.got) != 0 {
		t.Fatalf("enter on a disabled item ran it: %v", r.got)
	}
	press(m, "a")
	if len(m.popups) != 0 || strings.Join(r.got, ",") != "alpha" {
		t.Errorf("the shortcut should run alpha: popups=%d ran=%v", len(m.popups), r.got)
	}
}

func TestMenuEscAndQClose(t *testing.T) {
	for _, k := range []string{"esc", "q"} {
		m := newTestModel(t)
		m.push(newMenu("Test", []menuItem{{label: "a"}}))
		press(m, k)
		if len(m.popups) != 0 {
			t.Errorf("%s should close the menu", k)
		}
	}
}

func TestMultiMenu(t *testing.T) {
	m := newTestModel(t)
	var got []string
	menu := newMenu("Pick", []menuItem{{label: "ios"}, {label: "android", checked: true}, {label: "webgl"}})
	menu.multi = true
	menu.submit = func(m *Model, picked []menuItem) tea.Cmd {
		for _, p := range picked {
			got = append(got, p.label)
		}
		return nil
	}
	m.push(menu)
	if !strings.Contains(screenText(m), "[x] android") || !strings.Contains(screenText(m), "[ ] ios") {
		t.Errorf("checkboxes:\n%s", screenText(m))
	}
	press(m, "space") // tick ios
	press(m, "j", "j", "space")
	press(m, "j") // untick? cursor is on webgl; move to android and untick it
	press(m, "k", "space")
	press(m, "enter")
	if len(m.popups) != 0 {
		t.Error("enter should submit and close")
	}
	if strings.Join(got, ",") != "ios,webgl" {
		t.Errorf("picked = %v, want ios and webgl", got)
	}
}

func TestMenuFilterAsYouType(t *testing.T) {
	m := newTestModel(t)
	var r ran
	menu := newMenu("Test", []menuItem{
		{label: "Group", heading: true}, r.item("open project"), r.item("close window"), r.item("open folder"),
	})
	menu.filterable = true
	m.push(menu)
	press(m, "o", "p", "e", "n")
	if menu.list.Len() != 2 {
		t.Fatalf("filter 'open' left %d items", menu.list.Len())
	}
	press(m, "backspace", "backspace", "backspace", "backspace")
	if menu.list.Len() != 4 {
		t.Errorf("clearing the filter left %d items", menu.list.Len())
	}
	press(m, "c", "l")
	press(m, "enter")
	if strings.Join(r.got, ",") != "close window" {
		t.Errorf("ran %v", r.got)
	}

	// esc clears the filter first, then closes
	m.push(menu)
	menu.filter.Set("x")
	menu.list.SetFilter("x")
	press(m, "esc")
	if len(m.popups) != 1 || menu.list.Filter() != "" {
		t.Errorf("first esc should only clear the filter: popups=%d filter=%q", len(m.popups), menu.list.Filter())
	}
	press(m, "esc")
	if len(m.popups) != 0 {
		t.Error("second esc should close")
	}

	// typed letters filter instead of navigating
	m.push(menu)
	press(m, "j")
	if menu.filter.String() != "j" {
		t.Errorf("j in a filterable menu is text, got %q", menu.filter.String())
	}
}

func TestMenuDescriptionBox(t *testing.T) {
	m := newTestModel(t)
	m.push(newMenu("Test", []menuItem{{label: "alpha", desc: "Explains what alpha does"}, {label: "beta"}}))
	if !strings.Contains(screenText(m), "Explains what alpha does") {
		t.Error("the selected item's description should be shown under the menu")
	}
	press(m, "j")
	if strings.Contains(screenText(m), "Explains what alpha does") {
		t.Error("the description follows the cursor")
	}
}

func TestEmptyMenu(t *testing.T) {
	m := newTestModel(t)
	menu := newMenu("Test", nil)
	menu.emptyMsg = "Nothing here"
	m.push(menu)
	if !strings.Contains(screenText(m), "Nothing here") {
		t.Error("an empty menu needs a message")
	}
	press(m, "enter", "j", "k", "end", "esc")
	if len(m.popups) != 0 {
		t.Error("esc closes an empty menu")
	}
}

func TestPopupIsCenteredOverTheLayout(t *testing.T) {
	m := newTestModel(t)
	base := screen(m)
	m.push(&confirmPopup{title: "Sure?", message: "Really do it?"})
	got := screen(m)
	if len(got) != len(base) {
		t.Fatalf("the popup changed the screen height: %d vs %d", len(got), len(base))
	}
	for i, l := range got {
		if width(l) != 120 {
			t.Errorf("line %d is %d wide", i, width(l))
		}
	}
	row := -1
	for i, l := range got {
		if strings.Contains(l, "Really do it?") {
			row = i
		}
	}
	if row < 0 {
		t.Fatal("the message is not on screen")
	}
	if row < 12 || row > 24 {
		t.Errorf("the popup should be near the vertical centre, text is on row %d of %d", row, len(got))
	}
	// the message starts just inside the box's left border; the box is centred in 120 cells
	col := width(got[row][:strings.Index(got[row], "Really")])
	box := m.popups[0].view(m)
	left := (120 - width(strings.Split(box, "\n")[0])) / 2
	if col != left+2 {
		t.Errorf("the message starts at cell %d, want %d (box at %d, centred)", col, left+2, left)
	}
	// the layout is still visible around it
	if got[0] != base[0] || got[len(got)-1] != base[len(base)-1] {
		t.Error("the layout outside the popup must be untouched")
	}
	if !strings.Contains(strings.Join(got, "\n"), "Space Shooter") {
		t.Error("the project list should still show behind the popup")
	}
}

func TestPopupsStack(t *testing.T) {
	m := newTestModel(t)
	m.push(&infoPopup{title: "Under", lines: []string{"under"}})
	m.push(&confirmPopup{title: "Over", message: "over"})
	out := screenText(m)
	if !strings.Contains(out, "Over") || !strings.Contains(out, "over") {
		t.Error("the top popup is drawn")
	}
	press(m, "esc")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Under") {
		t.Errorf("closing the top popup reveals the one below: popups=%d", len(m.popups))
	}
	// keys go to the top popup only: j does not move the list behind
	before := selectedProject(t, m).name()
	press(m, "j", "j")
	if selectedProject(t, m).name() != before {
		t.Error("keys leaked through a popup to the list")
	}
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		key     string
		yes, no bool
	}{
		{"enter", true, false}, {"y", true, false}, {"esc", false, true}, {"n", false, true}, {"q", false, true},
	}
	for _, c := range cases {
		m := newTestModel(t)
		var yes, no bool
		m.push(&confirmPopup{
			title: "Sure?", message: "A message",
			onYes: func(*Model) tea.Cmd { yes = true; return nil },
			onNo:  func(*Model) tea.Cmd { no = true; return nil },
		})
		press(m, c.key)
		if yes != c.yes || no != c.no || len(m.popups) != 0 {
			t.Errorf("%s: yes=%v no=%v popups=%d", c.key, yes, no, len(m.popups))
		}
	}
	// other keys do nothing, and a missing onNo is fine
	m := newTestModel(t)
	m.push(&confirmPopup{title: "Sure?", message: "x", onYes: func(*Model) tea.Cmd { return nil }})
	press(m, "x", "j")
	if len(m.popups) != 1 {
		t.Error("unrelated keys must not close the popup")
	}
	press(m, "esc")
	if len(m.popups) != 0 {
		t.Error("esc without onNo still closes")
	}
}

func TestConfirmWrapsLongMessages(t *testing.T) {
	m := newTestModel(t)
	m.push(&confirmPopup{title: "Sure?", message: strings.Repeat("word ", 60) + "\nsecond paragraph", danger: true, yes: "Delete"})
	out := screenText(m)
	if !strings.Contains(out, "second paragraph") || !strings.Contains(out, "enter: Delete") {
		t.Errorf("long message:\n%s", out)
	}
	for i, l := range screen(m) {
		if width(l) != 120 {
			t.Errorf("line %d is %d wide", i, width(l))
		}
	}
}

func TestPrompt(t *testing.T) {
	m := newTestModel(t)
	var got string
	m.push(&promptPopup{
		title: "Name", label: "Name the project", in: newInput("My"),
		validate: func(s string) string {
			if strings.Contains(s, "/") {
				return "No slashes, please"
			}
			return ""
		},
		onSubmit: func(m *Model, text string) tea.Cmd { got = text; return nil },
	})
	press(m, " ", "G", "a", "m", "e")
	press(m, "/") // invalid
	press(m, "enter")
	if got != "" || len(m.popups) != 1 || !strings.Contains(screenText(m), "No slashes, please") {
		t.Fatalf("invalid input must stay open with the reason: got=%q popups=%d", got, len(m.popups))
	}
	press(m, "backspace") // typing clears the error
	if strings.Contains(screenText(m), "No slashes") {
		t.Error("the error should go away once the text changes")
	}
	press(m, "enter")
	if got != "My Game" || len(m.popups) != 0 {
		t.Errorf("submitted %q, popups=%d", got, len(m.popups))
	}
}

func TestPromptEscAndPasteAndComplete(t *testing.T) {
	m := newTestModel(t)
	called := false
	p := &promptPopup{
		title: "Path", in: newInput(""),
		complete: func(s string) []string { return []string{s + "/completed"} },
		onSubmit: func(*Model, string) tea.Cmd { called = true; return nil },
	}
	m.push(p)
	m.Update(tea.PasteMsg{Content: "/tmp/a\nb"})
	if p.in.String() != "/tmp/a b" {
		t.Errorf("paste: %q", p.in.String())
	}
	press(m, "tab")
	if p.in.String() != "/tmp/a b/completed" {
		t.Errorf("tab completion: %q", p.in.String())
	}
	press(m, "esc")
	if called || len(m.popups) != 0 {
		t.Error("esc cancels without submitting")
	}
}

func TestInfoPopupScrolls(t *testing.T) {
	m := newTestModel(t)
	var lines []string
	for i := 0; i < 80; i++ {
		lines = append(lines, "line number "+strings.Repeat("x", i%5)+string(rune('A'+i%26)))
	}
	info := &infoPopup{title: "Info", lines: lines}
	m.push(info)
	_ = screenText(m) // draws, and learns its height
	if info.height < 3 || info.height > 30 {
		t.Fatalf("height = %d", info.height)
	}
	press(m, "j", "j")
	if info.offset != 2 {
		t.Errorf("offset = %d", info.offset)
	}
	press(m, "k")
	if info.offset != 1 {
		t.Errorf("offset = %d", info.offset)
	}
	press(m, "pgdown")
	if info.offset <= 1 {
		t.Error("pgdown")
	}
	press(m, "end")
	if info.offset != len(lines)-info.height {
		t.Errorf("end: offset %d, want %d", info.offset, len(lines)-info.height)
	}
	press(m, "j") // cannot scroll past the end
	if info.offset != len(lines)-info.height {
		t.Error("scrolled past the end")
	}
	press(m, "home")
	if info.offset != 0 {
		t.Error("home")
	}
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if info.offset != 3 {
		t.Errorf("wheel: %d", info.offset)
	}
	press(m, "esc")
	if len(m.popups) != 0 {
		t.Error("esc closes")
	}
}

func TestKeybindingsMenuShowsLocalGlobalAndNavigation(t *testing.T) {
	m := newTestModel(t)
	press(m, "?")
	if len(m.popups) != 1 {
		t.Fatal("? opens the menu")
	}
	menu := m.popups[0].(*menuPopup)
	var labels []string
	for _, it := range menu.list.Items() {
		labels = append(labels, it.label)
	}
	all := strings.Join(labels, "\n")
	for _, want := range []string{
		"Local", "Global", "Navigation", // the sections
		"Open in Unity, signed in", "Change the Unity version", "Create a new project", // local to Projects
		"Quit", "Filter the list", "Show the command log", // global
		"Next panel", "Next tab of the panel", // navigation
	} {
		if !strings.Contains(all, want) {
			t.Errorf("the menu should list %q", want)
		}
	}
	if strings.Contains(all, "Install this version") || strings.Contains(all, "Show the Editor's modules") {
		t.Error("the menu lists the focused panel's actions, not every panel's")
	}
	// the screen shows the top of it, with the local section first
	out := screenText(m)
	if !strings.Contains(out, "Keybindings") || !strings.Contains(out, "─── Local") {
		t.Errorf("the menu should open on the local section:\n%s", out)
	}
	press(m, "q", "u", "i", "t")
	if menu.list.Len() != 1 {
		t.Fatalf("filter 'quit' left %d rows", menu.list.Len())
	}
	press(m, "enter") // runs the selected action: Quit
	if !m.quit {
		t.Error("choosing Quit in the menu should quit")
	}
}

func TestKeybindingsMenuFollowsTheFocusedPanel(t *testing.T) {
	m := newTestModel(t)
	press(m, "2", "?")
	menu := m.popups[0].(*menuPopup)
	var all []string
	for _, it := range menu.list.Items() {
		all = append(all, it.label)
	}
	joined := strings.Join(all, "\n")
	if !strings.Contains(joined, "Install a new Editor version") || strings.Contains(joined, "Change the Unity version") {
		t.Errorf("Editors panel menu:\n%s", joined)
	}
}

func TestKeybindingsMenuRunsActionsAfterClosing(t *testing.T) {
	m := newTestModel(t)
	press(m, "?")
	menu := m.popups[0].(*menuPopup)
	menu.filter.Set("Filter the list")
	menu.list.SetFilter("Filter the list")
	press(m, "enter")
	if len(m.popups) != 0 || !m.filter.active {
		t.Errorf("the action runs once the menu is gone: popups=%d filtering=%v", len(m.popups), m.filter.active)
	}
}

func TestCtrlCQuitsFromAPopup(t *testing.T) {
	m := newTestModel(t)
	m.push(&confirmPopup{title: "x", message: "y"})
	press(m, "ctrl+c")
	if !m.quit {
		t.Error("ctrl+c must always get you out")
	}
}

func TestClickChoosesAMenuRow(t *testing.T) {
	m := newTestModel(t)
	var r ran
	m.push(newMenu("Pick", []menuItem{r.item("first"), r.item("second"), r.item("third")}))
	box := m.popups[0].view(m)
	bw, bh := len(strings.Split(box, "\n")[0]), len(strings.Split(box, "\n"))
	_ = bw
	_ = bh
	// find the row of "second" on screen
	for y, l := range screen(m) {
		if strings.Contains(l, "second") {
			x := strings.Index(l, "second")
			m.Update(tea.MouseClickMsg{X: x + 1, Y: y, Button: tea.MouseLeft})
			break
		}
	}
	if len(m.popups) != 0 || strings.Join(r.got, ",") != "second" {
		t.Errorf("click: popups=%d ran=%v", len(m.popups), r.got)
	}
}

func TestInputEditing(t *testing.T) {
	in := newInput("hello world")
	k := func(s string) { in.Key(keyMsg(s)) }
	k("ctrl+w")
	if in.String() != "hello " {
		t.Errorf("ctrl+w: %q", in.String())
	}
	k("home")
	k("X")
	if in.String() != "Xhello " {
		t.Errorf("insert at home: %q", in.String())
	}
	k("end")
	k("ctrl+u")
	if in.String() != "" {
		t.Errorf("ctrl+u: %q", in.String())
	}
	in.Set("abcdef")
	k("left")
	k("left")
	k("backspace")
	if in.String() != "abcef" { // the cursor was between d and e
		t.Errorf("backspace mid-text: %q", in.String())
	}
	k("ctrl+k")
	if in.String() != "abc" {
		t.Errorf("ctrl+k: %q", in.String())
	}
	in.Set("日本語")
	k("backspace")
	if in.String() != "日本" {
		t.Errorf("wide characters: %q", in.String())
	}
	if in.Key(keyMsg("up")) {
		t.Error("keys the field does not use must be reported as not handled")
	}
	// the view is exactly as wide as asked, even when the text is longer
	in.Set(strings.Repeat("x", 100))
	if got := in.View(20, true); width(got) != 20 {
		t.Errorf("view width = %d", width(got))
	}
	if got := in.View(20, false); width(got) != 20 {
		t.Errorf("plain view width = %d", width(got))
	}
}

func TestInfoPopupWrapsLongLinesInsteadOfCuttingThem(t *testing.T) {
	m := newTestModel(t)
	long := "/home/someone/Projects/with/a/very/long/path/that/goes/on/and/on/for/more/than/the/popup/is/wide/ProjectName"
	m.push(&infoPopup{title: "Path", lines: []string{"short", long, "after"}})
	out := screenText(m)
	// wrapped text is no longer one string: strip the box and the breaks to check
	// that the whole path is there
	if !strings.Contains(flatten(out), long) {
		t.Errorf("the whole path should be there, wrapped:\n%s", out)
	}
	if !strings.Contains(out, "after") {
		t.Error("lines after the long one are still shown")
	}
	for i, l := range screen(m) {
		if width(l) != 120 {
			t.Errorf("line %d is %d wide", i, width(l))
		}
	}
}

// flatten removes everything that wrapping and framing add: box characters,
// spaces and line breaks.
func flatten(s string) string {
	return strings.NewReplacer("│", "", " ", "", "\n", "").Replace(s)
}

func TestMenuStaysPutWhileItsDescriptionChanges(t *testing.T) {
	m := newTestModel(t)
	long := strings.Repeat("A long description that wraps over several lines of the box. ", 3)
	m.push(newMenu("Steady menu", []menuItem{
		{label: "No description"},
		{label: "Long", desc: long},
		{label: "Short", desc: "One line."},
	}))
	place := func() (top int, tipAt int) {
		top, tipAt = -1, -1
		for i, l := range screen(m) {
			if strings.Contains(l, "Steady menu") {
				top = i
			}
			if strings.Contains(l, "A long description") || strings.Contains(l, "One line.") {
				if tipAt < 0 {
					tipAt = i
				}
			}
		}
		return top, tipAt
	}
	top0, tip0 := place()
	if top0 < 0 || tip0 != -1 {
		t.Fatalf("first item: menu at %d, description at %d:\n%s", top0, tip0, screenText(m))
	}
	press(m, "j")
	top1, tip1 := place()
	press(m, "j")
	top2, tip2 := place()
	if top1 != top0 || top2 != top0 {
		t.Errorf("the menu moved: %d, %d, %d", top0, top1, top2)
	}
	// the description box sits right under the menu: title, 3 items, bottom border, its own top border
	if tip1 != top0+5+1 || tip2 != tip1 {
		t.Errorf("descriptions at %d and %d, menu at %d:\n%s", tip1, tip2, top0, screenText(m))
	}
	// a click still lands on the row under the pointer
	pl := m.placePopup(m.popups[0])
	var chosen string
	menu := m.popups[0].(*menuPopup)
	items := menu.list.Items()
	for i := range items {
		label := items[i].label
		items[i].run = func(*Model) tea.Cmd { chosen = label; return nil }
	}
	menu.list.SetItems(items)
	m.Update(tea.MouseClickMsg{X: pl.x + 3, Y: pl.y + 1, Button: tea.MouseLeft})
	if chosen != "No description" {
		t.Errorf("clicked %q", chosen)
	}
}

func TestPromptShowsGhostCompletion(t *testing.T) {
	m := newTestModel(t)
	p := &promptPopup{
		title: "t", in: newInput("/tmp/a"),
		complete: func(s string) []string { return []string{s + "bc/"} },
	}
	if out := ansi.Strip(p.view(m)); !strings.Contains(out, "/tmp/abc/") {
		t.Errorf("the completion is drawn after the text:\n%s", out)
	}
	p.in.cur = 2 // cursor not at the end: no ghost
	if out := ansi.Strip(p.view(m)); strings.Contains(out, "/tmp/abc/") {
		t.Errorf("no ghost mid-text:\n%s", out)
	}
}

func TestRightArrowAcceptsGhostCompletion(t *testing.T) {
	m := newTestModel(t)
	p := &promptPopup{
		title: "t", in: newInput("/tmp/a"),
		complete: func(s string) []string { return []string{s + "bc/"} },
	}
	p.in.cur = 3
	p.update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if p.in.String() != "/tmp/a" || p.in.cur != 4 {
		t.Errorf("mid-text, right just moves: %q cur=%d", p.in.String(), p.in.cur)
	}
	p.in.cur = len(p.in.value)
	p.update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if p.in.String() != "/tmp/abc/" {
		t.Errorf("at the end, right accepts: %q", p.in.String())
	}
}

func TestMultiMenuGroupIsExclusive(t *testing.T) {
	m := newTestModel(t)
	menu := newMenu("Pick", []menuItem{
		{label: "plain"}, {label: "full", group: "g"}, {label: "core", group: "g"},
	})
	menu.multi = true
	m.push(menu)
	press(m, "j", "space") // full
	press(m, "j", "space") // core replaces full
	out := screenText(m)
	if !strings.Contains(out, "[x] core") || !strings.Contains(out, "[ ] full") {
		t.Errorf("one of the group:\n%s", out)
	}
	press(m, "space") // core off again
	if out := screenText(m); strings.Contains(out, "[x]") {
		t.Errorf("a group item can be unticked:\n%s", out)
	}
	press(m, "k", "k", "space") // plain does not touch the group
	press(m, "j", "space")
	if out := screenText(m); !strings.Contains(out, "[x] plain") || !strings.Contains(out, "[x] full") {
		t.Errorf("items outside the group stay:\n%s", out)
	}
}

func TestNewProjectOptionsManeTools(t *testing.T) {
	m := newTestModel(t)
	m.newProjectOptions(newProject{name: "G", parent: t.TempDir(), version: "6000.6.8f1"}, "3d")
	out := screenText(m)
	if !strings.Contains(out, "Add ManeTools (install OpenUPM to use)") || strings.Contains(out, "(full)") {
		t.Errorf("without OpenUPM there is one inactive option:\n%s", out)
	}
	m.popups = nil
	m.openupm = openupmState{checked: true, path: "/usr/bin/openupm", version: "3.0.0"}
	m.newProjectOptions(newProject{name: "G", parent: t.TempDir(), version: "6000.6.8f1"}, "3d")
	out = screenText(m)
	if !strings.Contains(out, "Add ManeTools (full)") || !strings.Contains(out, "Add ManeTools (w/o UI module)") || strings.Contains(out, "install OpenUPM") {
		t.Errorf("with OpenUPM there are two options:\n%s", out)
	}
}

// quits reports whether cmd ends the program.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); ok {
		return true
	}
	for _, c := range sequenceOf(msg) {
		if quits(c) {
			return true
		}
	}
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, c := range b {
			if quits(c) {
				return true
			}
		}
	}
	return false
}

// Quitting from a menu with the mouse quits at once, as it does with a key.
func TestClickingQuitInTheKeybindingsMenuQuits(t *testing.T) {
	m := newTestModel(t)
	press(m, "?")
	for y, l := range screen(m) {
		if i := strings.Index(l, " Quit "); i >= 0 && !strings.Contains(l, "Keybindings") {
			_, cmd := m.Update(tea.MouseClickMsg{X: i + 2, Y: y, Button: tea.MouseLeft})
			if !quits(cmd) {
				t.Fatalf("clicking Quit should quit:\n%s", screenText(m))
			}
			return
		}
	}
	t.Fatalf("no Quit row in the keybindings menu:\n%s", screenText(m))
}
