package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// Options is what the interface starts from. Data loaded before the first frame
// is passed in, so the first thing drawn already has projects on it.
type Options struct {
	Config    config.Config
	State     *state.State
	ConfigDir string
	Version   string // lazyunity's own version

	Client *unitycli.Client // nil when there is no usable CLI
	CLI    unitycli.Status
	// Env is the machine, for finding programs and reading the environment.
	// What it leaves out is taken from the real one.
	Env unitycli.Env
	// host starts programs; nil means the real machine. Tests set it.
	host host
	GOOS string
	Now  func() time.Time

	Projects []unitycli.Project
	// Local are the Editors found in the Editor folders. When nil, New looks
	// for them itself.
	Local       []unity.Installed
	InstallRoot string
	// Warnings are shown to the user once the interface is up.
	Warnings []string
}

type toastKind int

const (
	toastInfo toastKind = iota
	toastOK
	toastWarn
	toastError
)

type toast struct {
	text  string
	kind  toastKind
	until time.Time
}

// filterState is the '/' prompt.
type filterState struct {
	active bool
	in     *input
}

// Model is the whole interface.
type Model struct {
	cfg       config.Config
	st        *state.State
	th        *Theme
	keymap    *keymap
	configDir string
	version   string

	cli       unityCLI // nil when there is no usable CLI
	cliStatus unitycli.Status
	env       unitycli.Env
	host      host
	goos      string
	now       func() time.Time

	w, h   int
	mode   screenMode
	focus  int
	geo    geometry
	panels [numPanels]*panel

	projectsTab *listTab[projectRow]
	editorsTab  *listTab[editorRow]
	releasesTab *releasesTab
	toolsTab    *listTab[toolRow]

	// data
	rawProjects []unitycli.Project
	missing     map[string]bool
	inv         *inventory
	auth        unitycli.AuthInfo
	authKnown   bool
	cliUpdate   unitycli.UpdateInfo // what the launch check found; empty until it answers
	openupm     openupmState

	popups []popup
	toast  toast
	filter filterState
	quit   bool
	// restart asks main to start lazyunity again once it quits, to apply an edited config.
	restart bool
	// configPending is a config edited while a task ran, applied by a restart from the Tools tab.
	configPending bool

	// selectAfterLoad is a project path to put the cursor on once the list reloads.
	selectAfterLoad string

	tasks         []*task
	quitConfirmed bool
	nextTaskID    int
	spinning      bool
	spin          int
}

// New builds the interface.
func New(o Options) (*Model, error) {
	th, err := NewTheme(o.Config.GUI)
	if err != nil {
		return nil, err
	}
	m := &Model{
		cfg: o.Config, st: o.State, th: th, configDir: o.ConfigDir, version: o.Version,
		cliStatus: o.CLI, env: o.Env, goos: o.GOOS, now: o.Now,
		mode: parseScreenMode(o.Config.GUI.ScreenMode), missing: map[string]bool{},
	}
	if o.Client != nil { // a nil *Client in the interface would not be nil
		m.cli = o.Client
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.goos == "" {
		m.goos = o.Env.GOOS
	}
	real := unitycli.DefaultEnv()
	if m.env.GOOS == "" {
		m.env.GOOS = m.goos
	}
	if m.env.Getenv == nil {
		m.env.Getenv = real.Getenv
	}
	if m.env.LookPath == nil {
		m.env.LookPath = real.LookPath
	}
	if m.host = o.host; m.host == nil {
		m.host = machine{env: m.env}
	}
	if m.st == nil {
		m.st = state.Load("")
	}
	m.inv = newInventory(m.goos, homeDir(), o.InstallRoot)
	km, warnings := newKeymap(buildActions(), o.Config.Keybinding)
	m.keymap = km
	m.buildTabs()
	m.focus = panelProjects // always the first panel at launch: focus is not saved
	if !m.mode.shows(m.focus) {
		m.focus = map[bool]int{true: panelProjects, false: panelEditors}[m.mode == modeProjects]
	}

	if o.Local != nil {
		m.inv.setFound(withPlatforms(m.goos, o.Local))
	} else {
		m.inv.setFound(scanEditors(m.goos, m.inv.installRoot()))
	}
	m.rawProjects = o.Projects
	m.refreshEditors()
	m.refreshProjects()
	m.refreshTools()

	all := append(append([]string(nil), o.Warnings...), warnings...)
	if len(all) > 0 {
		m.toastf(toastWarn, "%s", strings.Join(all, "; "))
	}
	return m, nil
}

// buildTabs creates the panels and their lists.
func (m *Model) buildTabs() {
	m.projectsTab = &listTab[projectRow]{
		list:    newList(func(r projectRow) string { return r.name() + " " + r.Path + " " + r.Version }, func(r projectRow) string { return r.Path }),
		name:    "Projects",
		context: ctxProjects,
		empty:   "No projects. Press a to add one, n to create one.",
	}
	var pcols projectCols
	m.projectsTab.prep = func(m *Model, w int) { pcols = m.measureProjects(m.projectsTab.Items()) }
	m.projectsTab.draw = func(m *Model, r projectRow, w int, base lipgloss.Style) string {
		return m.drawProject(r, w, base, pcols)
	}

	m.editorsTab = &listTab[editorRow]{
		list:    newList(func(r editorRow) string { return r.Version + " " + r.platformLabels() }, editorRow.key),
		name:    "Installed Editors",
		context: ctxEditors,
		empty:   "No Unity Editors installed. Press n to install one.",
	}
	var ecols editorCols
	m.editorsTab.prep = func(m *Model, w int) { ecols = measureEditors(m.editorsTab.Items()) }
	m.editorsTab.draw = func(m *Model, r editorRow, w int, base lipgloss.Style) string {
		return m.drawEditor(r, w, base, ecols)
	}

	m.releasesTab = newReleasesTab(m.st.Stream())

	m.toolsTab = &listTab[toolRow]{
		list: newList(func(r toolRow) string { return r.label }, func(r toolRow) string { return r.id }).
			skipping(func(r toolRow) bool { return r.spacer }),
		name:    "Tools",
		context: ctxTools,
	}
	m.toolsTab.draw = func(m *Model, r toolRow, w int, base lipgloss.Style) string { return m.drawTool(r, w, base) }

	m.panels = [numPanels]*panel{
		panelProjects: {jump: "1", tabs: []tab{m.projectsTab}},
		panelEditors:  {jump: "2", tabs: []tab{m.editorsTab, m.releasesTab}},
		panelOptions:  {jump: "3", tabs: []tab{m.toolsTab}},
	}
}

// ---- small accessors ----------------------------------------------------------

func (m *Model) focusedPanel() *panel { return m.panels[m.focus] }
func (m *Model) focusedTab() tab      { return m.focusedPanel().active() }
func (m *Model) focusCtx() ctxID      { return m.focusedTab().ctx() }

// editorInstalled reports whether an Editor with this version is installed.
func (m *Model) editorInstalled(version string) bool { return m.inv.has(version) }

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func (m *Model) toastf(kind toastKind, format string, args ...any) {
	d := 4 * time.Second
	if kind == toastError || kind == toastWarn {
		d = 8 * time.Second
	}
	m.toast = toast{text: fmt.Sprintf(format, args...), kind: kind, until: m.now().Add(d)}
}

func (m *Model) push(p popup) { m.popups = append(m.popups, p) }
func (m *Model) pop() {
	if n := len(m.popups); n > 0 {
		m.popups = m.popups[:n-1]
	}
}

// ---- layout ------------------------------------------------------------------

// relayout recomputes the geometry and tells every list its height.
func (m *Model) relayout() {
	m.geo = computeGeometry(m.w, m.h, m.cfg.GUI.SidePanelWidth, m.mode, m.tasksHeight(), m.cfg.GUI.ShowBottomLine)
	for i, p := range m.panels {
		if r := m.geo.panels[i]; !r.empty() {
			p.setHeight(r.h - 2)
		}
	}
}

// setFocus focuses a panel. A panel the screen mode hides is brought on screen
// by switching to a mode that shows it.
func (m *Model) setFocus(i int) {
	i = clamp(i, 0, numPanels-1)
	if m.filter.active {
		m.filter.active = false
	}
	m.focus = i
	if !m.mode.shows(i) {
		m.mode = modeShowing(i)
		m.relayout()
	}
}

// ---- tea.Model -----------------------------------------------------------------

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Restart reports whether lazyunity quit to be started again, with the config reloaded.
func (m *Model) Restart() bool { return m.restart }

// Init starts the slow loads; what was cheap to load is already in place.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(tick(), m.probeOpenUPMCmd(), m.loadEditorsCmd(), m.loadAuthCmd(), m.loadInstallPathCmd(), m.checkCLIUpdateCmd(), queryPaletteCmd())
}

// Update handles every message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.relayout()
	case tickMsg:
		cmd = tick()
	case tea.KeyPressMsg:
		cmd = m.handleKey(msg)
	case tea.PasteMsg:
		cmd = m.handlePaste(msg)
	case tea.MouseClickMsg:
		cmd = m.handleClick(msg)
	case tea.MouseWheelMsg:
		cmd = m.handleWheel(msg)
	case tea.FocusMsg:
		cmd = m.loadProjectsCmd()
	case uv.UnknownOscEvent:
		m.th.setPaletteFromOSC(string(msg))
	default:
		var ok bool
		if cmd, ok = m.handleTask(msg); !ok {
			cmd = m.handleData(msg)
		}
	}
	return m, m.finish(cmd)
}

// finish runs after every update, any of which may have asked to quit.
func (m *Model) finish(cmd tea.Cmd) tea.Cmd {
	if m.quit {
		return tea.Sequence(cmd, tea.Quit)
	}
	return cmd
}

// toPopup hands a message to the popup on top.
func (m *Model) toPopup(msg tea.Msg) tea.Cmd {
	return m.resolve(m.popups[len(m.popups)-1].update(m, msg))
}

// resolve acts on what a popup answered: it closes the popup when it is done,
// then runs next, which may open another one.
func (m *Model) resolve(next func(*Model) tea.Cmd, done bool) tea.Cmd {
	if done {
		m.pop()
	}
	if next != nil {
		return next(m)
	}
	return nil
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "ctrl+c" && len(m.popups) > 0 {
		return m.requestQuit()
	}
	if len(m.popups) > 0 {
		return m.toPopup(msg)
	}
	if m.filter.active {
		return m.updateFilter(msg)
	}
	a := m.keymap.lookup(m.focusCtx(), key)
	if a == nil || !a.available(m) {
		return nil
	}
	return a.run(m)
}

func (m *Model) handlePaste(msg tea.PasteMsg) tea.Cmd {
	if len(m.popups) > 0 {
		return m.toPopup(msg)
	}
	if m.filter.active {
		m.filter.in.Paste(msg.Content)
		m.focusedTab().setFilter(m.filter.in.String())
		return m.focusedTab().filtered(m)
	}
	return nil
}

// requestQuit quits, after asking when something would be lost by it.
func (m *Model) requestQuit() tea.Cmd {
	if m.busy() && !m.quitConfirmed {
		m.push(&confirmPopup{
			title: "Quit", yes: "Quit", danger: true,
			message: "A task is still running. Quitting cancels it.",
			onYes: func(m *Model) tea.Cmd {
				m.quitConfirmed = true
				m.cancelTasks()
				m.quit = true
				return nil
			},
		})
		return nil
	}
	m.quit = true
	return nil
}

// ---- filter ---------------------------------------------------------------------

func (m *Model) startFilter() {
	m.filter = filterState{active: true, in: newInput(m.focusedTab().filter())}
}

func (m *Model) updateFilter(msg tea.KeyPressMsg) tea.Cmd {
	t := m.focusedTab()
	switch msg.String() {
	case "esc":
		t.setFilter("")
		m.filter.active = false
	case "enter":
		m.filter.active = false
	case "up", "down":
		t.nav().Move(map[string]int{"up": -1, "down": 1}[msg.String()])
		return m.afterNav()
	default:
		if m.filter.in.Key(msg) {
			t.setFilter(m.filter.in.String())
			return t.filtered(m)
		}
	}
	return nil
}

// ---- mouse --------------------------------------------------------------------

func (m *Model) handleClick(msg tea.MouseClickMsg) tea.Cmd {
	if !m.cfg.GUI.MouseEvents || msg.Button != tea.MouseLeft {
		return nil
	}
	if n := len(m.popups); n > 0 {
		return m.popupClick(msg)
	}
	i := m.geo.panelAt(msg.X, msg.Y)
	if i < 0 {
		return nil
	}
	m.setFocus(i)
	r := m.geo.panels[i]
	p := m.panels[i]
	if msg.Y == r.y { // the top border: tab names
		if ti := p.tabAtX(msg.X-r.x, m.cfg.GUI.ShowPanelJumps); ti >= 0 {
			p.cur = ti
			return m.afterTabSwitch()
		}
		return nil
	}
	if msg.Y == r.y+r.h-1 {
		return nil
	}
	row := msg.Y - r.y - 1
	if t := p.active(); t.filterRow() && row == r.h-3 {
		m.startFilter()
		return nil
	}
	p.active().click(row)
	return nil
}

func (m *Model) handleWheel(msg tea.MouseWheelMsg) tea.Cmd {
	if !m.cfg.GUI.MouseEvents {
		return nil
	}
	if len(m.popups) > 0 {
		return m.toPopup(msg)
	}
	i := m.geo.panelAt(msg.X, msg.Y)
	if i < 0 {
		return nil
	}
	switch msg.Button {
	case tea.MouseWheelUp:
		m.panels[i].active().scroll(-3)
	case tea.MouseWheelDown:
		m.panels[i].active().scroll(3)
	}
	return nil
}

// popupClick hands a click inside the top popup's border to it, as the row
// of the box that was clicked.
func (m *Model) popupClick(msg tea.MouseClickMsg) tea.Cmd {
	p := m.popups[len(m.popups)-1]
	pl := m.placePopup(p)
	bw, _ := lipgloss.Size(pl.box)
	row := msg.Y - pl.y - 1
	if msg.X <= pl.x || msg.X >= pl.x+bw-1 || row < 0 {
		return nil
	}
	return m.resolve(p.click(m, row))
}

// ---- view ---------------------------------------------------------------------

// View draws the interface.
func (m *Model) View() tea.View {
	var content string
	switch {
	case m.w == 0:
		content = ""
	case !m.geo.ok:
		content = m.tooSmallView()
	default:
		content = m.compose()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "lazyunity"
	v.ReportFocus = true
	if m.cfg.GUI.MouseEvents {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

func (m *Model) tooSmallView() string {
	msg := "Not enough space to render panels"
	hint := fmt.Sprintf("need at least %dx%d", minScreenW, minScreenH)
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, msg+"\n"+m.th.Dim.Render(hint))
}

// compose draws the panels, the task frame and the hint line, then the popups.
func (m *Model) compose() string {
	cols := m.panelsView()
	parts := []string{cols}
	if !m.geo.tasks.empty() {
		parts = append(parts, m.tasksView(m.geo.tasks))
	}
	if !m.geo.bar.empty() {
		parts = append(parts, m.barView(m.geo.bar.w))
	}
	base := strings.Join(parts, "\n")
	if len(m.popups) == 0 {
		return base
	}
	layers := make([]*lipgloss.Layer, 0, len(m.popups)+1)
	layers = append(layers, lipgloss.NewLayer(base))
	for i, p := range m.popups {
		pl := m.placePopup(p)
		layers = append(layers, lipgloss.NewLayer(pl.box).X(pl.x).Y(pl.y).Z(i+1))
		if pl.below != "" {
			_, bh := lipgloss.Size(pl.box)
			layers = append(layers, lipgloss.NewLayer(pl.below).X(pl.x).Y(pl.y+bh).Z(i+1))
		}
	}
	return lipgloss.NewCompositor(layers...).Render()
}

// placedPopup is a popup drawn and given its place on screen: the box at x, y
// and, right under it, what goes below (a menu item's description).
type placedPopup struct {
	box, below string
	x, y       int
}

// placePopup centres a popup. A menu is centred as if its tallest description
// box were always under it, so it does not move as the selection changes.
func (m *Model) placePopup(p popup) placedPopup {
	var pl placedPopup
	var reserve int
	pl.box, pl.below, reserve = p.layers(m)
	bw, bh := lipgloss.Size(pl.box)
	pl.x = max((m.w-bw)/2, 0)
	pl.y = max((m.h-bh-reserve)/2, 0)
	return pl
}

// panelsView draws the panel frames in their places.
func (m *Model) panelsView() string {
	g := m.geo
	frames := [numPanels]string{}
	for i := range m.panels {
		if !g.panels[i].empty() {
			frames[i] = m.renderPanel(i, g.panels[i])
		}
	}
	switch m.mode {
	case modeProjects:
		return frames[panelProjects]
	case modeEditors:
		return lipgloss.JoinVertical(lipgloss.Left, frames[panelEditors], frames[panelOptions])
	}
	right := lipgloss.JoinVertical(lipgloss.Left, frames[panelEditors], frames[panelOptions])
	return lipgloss.JoinHorizontal(lipgloss.Top, frames[panelProjects], right)
}

func (m *Model) renderPanel(i int, r rect) string {
	p := m.panels[i]
	t := p.active()
	focused := i == m.focus
	inner := r.w - 2

	var lines []string
	if t.nav().Len() == 0 {
		msg := t.emptyMsg()
		if f := t.filter(); f != "" {
			msg = fmt.Sprintf("No matches for '%s'", f)
			if t.loading() {
				msg += " yet. Still looking…"
			}
		}
		for _, l := range wrapText(msg, max(inner-2, 4)) {
			lines = append(lines, " "+m.th.Dim.Render(l))
		}
	} else {
		lines = t.rows(m, inner, focused)
	}
	if t.filterRow() && r.h > 3 {
		rows := r.h - 3
		lines = lines[:min(len(lines), rows)]
		if t.loading() && rows > 0 {
			// "loading more" sits right under the last item
			more := " " + m.th.ActiveBorder.Render(m.spinnerFrame()) + m.th.Dim.Render(" loading more")
			if len(lines) < rows {
				lines = append(lines, more)
			} else {
				lines[rows-1] = more
			}
		}
		for len(lines) < rows {
			lines = append(lines, "")
		}
		lines = append(lines, m.filterRowView(t, inner, focused))
	}
	names, cur := p.titles()
	jump, footer := "", ""
	if m.cfg.GUI.ShowPanelJumps {
		jump = p.jump
	}
	if m.cfg.GUI.ShowListFooter {
		footer = t.footer()
	}
	return m.th.renderFrame(frame{
		w: r.w, h: r.h, jump: jump, tabs: names, tab: cur,
		focused: focused, searching: focused && m.filter.active,
		lines: lines, footer: footer, scroll: t.scrollbar(),
	})
}

// filterRowView is the filter row at the bottom of a tab that has one: the
// filter, and on the right whether more is loading.
func (m *Model) filterRowView(t tab, w int, focused bool) string {
	th := m.th
	right := ""
	if s := t.status(); s != "" {
		right = th.Dim.Render(s)
	}
	label := " Filter: "
	avail := max(w-width(label)-width(right)-1, 2)
	var field string
	switch {
	case focused && m.filter.active:
		field = m.filter.in.View(avail)
	case t.filter() != "":
		field = th.Warning.Render(padRight(truncate(t.filter(), avail), avail))
	default:
		field = th.Dim.Render(padRight(truncate("press / to type a version", avail), avail))
	}
	return label + field + " " + right
}

// barView is the line at the bottom: the keys on the left, status on the right.
func (m *Model) barView(w int) string {
	if m.filter.active && !m.focusedTab().filterRow() {
		label := "Filter: "
		return label + m.filter.in.View(w-width(label))
	}
	if m.filter.active { // typed in the tab's own filter row
		return renderHints(m.th, []hintItem{{"Done", "<enter>"}, {"Clear", "<esc>"}, {"Move", "<up>/<down>"}}, w)
	}
	right := m.statusView()
	if m.version != "" { // the app's own version ends the line
		right += m.th.Dim.Render(" · ") + m.th.Text.Render(m.version)
	}
	rw := width(right)
	left := ""
	if avail := w - rw - 1; avail > 4 {
		left = renderHints(m.th, m.keymap.hints(m, m.focusCtx()), avail)
	}
	gap := max(w-width(left)-rw, 0)
	return truncate(left+strings.Repeat(" ", gap)+right, w)
}

// statusView is the right-hand side of the bottom line.
func (m *Model) statusView() string {
	th := m.th
	if m.now().Before(m.toast.until) {
		st := th.Text
		switch m.toast.kind {
		case toastOK:
			st = th.Success
		case toastWarn:
			st = th.Warning
		case toastError:
			st = th.Missing
		}
		return st.Render(truncate(m.toast.text, max(m.w/2, 20)))
	}
	if f := m.focusedTab().filter(); f != "" && !m.focusedTab().filterRow() {
		return th.Warning.Render("filter: "+truncate(f, 20)) + th.Dim.Render("  esc: clear")
	}
	var parts []string
	switch m.cliStatus.State {
	case unitycli.Missing:
		return th.Warning.Render("Unity CLI not installed")
	case unitycli.Outdated:
		parts = append(parts, th.Warning.Render("Unity CLI outdated"))
	default:
		switch {
		case !m.authKnown:
			parts = append(parts, th.Dim.Render("…"))
		case m.auth.LoggedIn:
			parts = append(parts, th.Success.Render("● signed in"))
		default:
			parts = append(parts, th.Warning.Render("○ signed out"))
		}
	}
	return strings.Join(parts, th.Dim.Render(" · "))
}
