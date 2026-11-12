package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

type taskStatus int

const (
	taskQueued taskStatus = iota
	taskRunning
	taskDone
)

// task is a long-running job: an install, a download, a project being created.
// They run one at a time, because the CLI holds a lock while it installs.
type task struct {
	id    int
	title string
	// label is the short name a task has in a list of several, such as
	// "6000.3.28f1" or "6000.3.28f1: Android Build Support". Empty: the title.
	label string
	// works are what the task works on, such as "module:6000.3.28f1:android".
	// A task is refused while another one works on any of the same things.
	works  []string
	status taskStatus
	// pct is the progress of the current phase, 0 to 100; negative when unknown.
	pct   float64
	phase string
	msg   string
	err   error

	// A task is either a stream (installs, which report progress) or a plain
	// blocking call (which can only spin).
	open func(ctx context.Context) (*unitycli.Stream, error)
	do   func(ctx context.Context) error
	// after runs when the task ends, whatever the outcome.
	after func(m *Model, err error) tea.Cmd

	cancel context.CancelFunc
	// cancelled is set when the user stopped the task. How a stopped process
	// fails depends on the program and the platform (an exit code, a signal,
	// a killed process on Windows), so the error cannot tell.
	cancelled bool
	doneAt    time.Time
}

// ---- messages ------------------------------------------------------------------

type taskFrameMsg struct {
	id     int
	frame  unitycli.Frame
	stream *unitycli.Stream
}

type taskEndMsg struct {
	id  int
	err error
}

type taskStartedMsg struct {
	id     int
	stream *unitycli.Stream
	err    error
}

type spinMsg struct{}

// cliDoneMsg is the end of a quick CLI call started with runCLI.
type cliDoneMsg struct {
	what  string
	err   error
	ok    string
	after func(m *Model) tea.Cmd
}

// ---- the queue -----------------------------------------------------------------

const (
	spinInterval  = 120 * time.Millisecond
	doneLinger    = 2 * time.Second
	maxTaskLines  = 3
	spinnerFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
)

// refuseWhileBusy tells the user to wait, and reports true, while tasks are
// queued or running. The installers run with the terminal handed over, which
// freezes lazyunity and the progress of every task, and the Unity CLI
// installer replaces the program the tasks are using.
func (m *Model) refuseWhileBusy(what string) bool {
	if !m.busy() {
		return false
	}
	m.toastf(toastWarn, "Wait for the running tasks to finish before %s", what)
	return true
}

func (m *Model) busy() bool {
	for _, t := range m.tasks {
		if t.status == taskQueued || t.status == taskRunning {
			return true
		}
	}
	return false
}

func (m *Model) running() *task {
	for _, t := range m.tasks {
		if t.status == taskRunning {
			return t
		}
	}
	return nil
}

func (m *Model) taskByID(id int) *task {
	for _, t := range m.tasks {
		if t.id == id {
			return t
		}
	}
	return nil
}

// shortLabel is how a task is named in a list of several.
func (t *task) shortLabel() string {
	if t.label != "" {
		return t.label
	}
	return t.title
}

// taskOn returns the task that works on w, if one is queued, running, or has
// just finished (its result may not be on screen yet). A task that failed or
// was cancelled is gone from the list.
func (m *Model) taskOn(w string) *task {
	for _, t := range m.tasks {
		for _, x := range t.works {
			if x == w {
				return t
			}
		}
	}
	return nil
}

// Keys for task.works.
func workEditor(version string) string        { return "editor:" + version }
func workModule(version, id string) string    { return "module:" + version + ":" + id }
func workProject(path string) string          { return "project:" + path }
func workUpgradeEditor(version string) string { return "upgrade:" + version }

// enqueue adds a task and starts it if nothing else is running. A task that
// would work on something another task is already busy with is refused.
func (m *Model) enqueue(t *task) tea.Cmd {
	for _, w := range t.works {
		if other := m.taskOn(w); other != nil {
			m.toastf(toastInfo, "Already in the queue: %s", other.title)
			return nil
		}
	}
	m.nextTaskID++
	t.id = m.nextTaskID
	t.pct = -1
	t.status = taskQueued
	m.tasks = append(m.tasks, t)
	m.relayout()
	return tea.Batch(m.startNext(), m.startSpinner())
}

func spin() tea.Cmd {
	return tea.Tick(spinInterval, func(time.Time) tea.Msg { return spinMsg{} })
}

// startSpinner starts the animation clock, unless it is already running. It
// runs while there are tasks or a tab is loading.
func (m *Model) startSpinner() tea.Cmd {
	if m.spinning {
		return nil
	}
	m.spinning = true
	return spin()
}

func (m *Model) spinnerFrame() string {
	frames := []rune(spinnerFrames)
	return string(frames[m.spin%len(frames)])
}

// startNext starts the first queued task when none is running.
func (m *Model) startNext() tea.Cmd {
	if m.running() != nil {
		return nil
	}
	for _, t := range m.tasks {
		if t.status != taskQueued {
			continue
		}
		t.status = taskRunning
		ctx, cancel := context.WithCancel(context.Background())
		t.cancel = cancel
		id := t.id
		switch {
		case t.open != nil:
			open := t.open
			return func() tea.Msg {
				s, err := open(ctx)
				return taskStartedMsg{id: id, stream: s, err: err}
			}
		case t.do != nil:
			do := t.do
			return func() tea.Msg { return taskEndMsg{id: id, err: do(ctx)} }
		}
		t.status = taskDone
	}
	return nil
}

// waitFrame waits for the next frame of a stream, or for its end.
func waitFrame(id int, s *unitycli.Stream) tea.Cmd {
	return func() tea.Msg {
		f, ok := <-s.Frames
		if !ok {
			return taskEndMsg{id: id, err: s.Wait()}
		}
		return taskFrameMsg{id: id, frame: f, stream: s}
	}
}

// handleTask takes in the messages of the queue. ok is false for other messages.
func (m *Model) handleTask(msg tea.Msg) (cmd tea.Cmd, ok bool) {
	switch msg := msg.(type) {
	case taskStartedMsg:
		if msg.err != nil {
			return m.onTaskEnd(taskEndMsg{id: msg.id, err: msg.err}), true
		}
		return waitFrame(msg.id, msg.stream), true
	case taskFrameMsg:
		t := m.taskByID(msg.id)
		if t == nil {
			return nil, true
		}
		if f := msg.frame; !f.IsResult() {
			if f.Pct != nil {
				t.pct = *f.Pct
			}
			if f.Phase != "" {
				t.phase = f.Phase
			}
			if f.Message != "" {
				t.msg = f.Message
			}
		}
		return waitFrame(msg.id, msg.stream), true
	case taskEndMsg:
		return m.onTaskEnd(msg), true
	case spinMsg:
		m.spin++
		m.pruneTasks()
		if len(m.tasks) == 0 && !m.tabsLoading() {
			m.spinning = false
			return nil, true
		}
		return spin(), true
	case cliDoneMsg:
		return m.onCLIDone(msg), true
	}
	return nil, false
}

func (m *Model) onTaskEnd(msg taskEndMsg) tea.Cmd {
	t := m.taskByID(msg.id)
	if t == nil {
		return nil
	}
	if t.cancel != nil {
		t.cancel()
	}
	t.err = msg.err
	t.doneAt = m.now()
	var cmds []tea.Cmd
	switch {
	case msg.err == nil:
		t.status = taskDone
		t.pct = 100
	case t.cancelled || isCancelled(msg.err):
		m.removeTask(t)
		m.toastf(toastInfo, "Cancelled: %s", t.title)
	default:
		m.removeTask(t)
		m.showError(t.title, msg.err)
	}
	if t.after != nil {
		cmds = append(cmds, t.after(m, msg.err))
	}
	cmds = append(cmds, m.startNext())
	m.relayout()
	return tea.Batch(cmds...)
}

func isCancelled(err error) bool {
	var e *unitycli.Error
	return errors.Is(err, context.Canceled) || (errors.As(err, &e) && e.Cancelled())
}

func (m *Model) removeTask(t *task) {
	for i, x := range m.tasks {
		if x == t {
			m.tasks = append(m.tasks[:i], m.tasks[i+1:]...)
			return
		}
	}
}

// pruneTasks drops finished tasks once they have been shown for a moment.
func (m *Model) pruneTasks() {
	changed := false
	kept := m.tasks[:0]
	for _, t := range m.tasks {
		if t.status == taskDone && m.now().Sub(t.doneAt) >= doneLinger {
			changed = true
			continue
		}
		kept = append(kept, t)
	}
	m.tasks = kept
	if changed {
		m.relayout()
	}
}

// cancelRunning stops the running task. The queued ones stay: the next starts
// once the running one has ended.
func (m *Model) cancelRunning() {
	if t := m.running(); t != nil {
		t.stop()
	}
}

// stop cancels a running task.
func (t *task) stop() {
	if t.cancel != nil {
		t.cancelled = true
		t.cancel()
	}
}

// cancelTasks stops the running task and drops the queued ones, for quitting.
func (m *Model) cancelTasks() {
	for _, t := range append([]*task(nil), m.tasks...) {
		switch t.status {
		case taskQueued:
			m.removeTask(t)
		case taskRunning:
			t.stop()
		}
	}
	m.relayout()
}

// tasksHeight is the height of the frame at the bottom: one line per task
// shown, plus the border. Zero when there are no tasks.
func (m *Model) tasksHeight() int {
	if len(m.tasks) == 0 {
		return 0
	}
	return min(len(m.tasks), maxTaskLines) + 2
}

// ---- drawing -------------------------------------------------------------------

func (m *Model) tasksView(r rect) string {
	th := m.th
	inner := r.w - 2
	shown := m.tasks
	if len(shown) > maxTaskLines {
		shown = shown[:maxTaskLines]
	}
	// one task or many, the frame looks the same: "Tasks", then a labelled
	// line per task
	title := "Tasks"
	labelW := 0
	for _, t := range shown {
		labelW = max(labelW, width(t.shortLabel()))
	}
	labelW = min(labelW, max((inner-2)/2, 12))
	var lines []string
	for _, t := range shown {
		lines = append(lines, " "+m.taskLine(t, inner-2, labelW))
	}
	footer := ""
	if extra := len(m.tasks) - len(shown); extra > 0 {
		footer = fmt.Sprintf("+%d more", extra)
	}
	return th.renderFrame(frame{w: r.w, h: r.h, tabs: []string{title}, focused: true, lines: lines, footer: footer})
}

// indeterminate reports whether a task's progress cannot be told: nothing
// reported yet, or the install phase, where the CLI holds the percentage at 50
// until the installer is done.
func (t *task) indeterminate() bool {
	return t.pct < 0 || (t.phase == "install" && t.pct < 100)
}

// taskLine draws one task: icon, label (when labelW > 0), bar,
// percentage, what it is doing.
func (m *Model) taskLine(t *task, w int, labelW int) string {
	th := m.th
	var icon string
	switch t.status {
	case taskRunning:
		icon = th.ActiveBorder.Render(m.spinnerFrame())
	case taskDone:
		icon = th.Success.Render("✓")
	default:
		icon = th.Dim.Render("○")
	}
	var parts []string
	used := 2
	if labelW > 0 {
		parts = append(parts, fit(t.shortLabel(), labelW))
		used += labelW + 1
	}
	if t.status == taskQueued {
		return icon + " " + strings.Join(parts, " ") + th.Dim.Render(" queued")
	}
	pct := ""
	if !t.indeterminate() || t.status == taskDone {
		pct = fmt.Sprintf("%3.0f%%", t.pct)
	}
	barW := clamp((w-used)/3, 10, 32)
	parts = append(parts, m.bar(t, barW))
	used += barW + 1
	if pct != "" {
		parts = append(parts, pct)
		used += width(pct) + 1
	}
	detail := t.msg
	if t.phase != "" && t.msg != "" {
		detail = t.phase + " · " + t.msg
	} else if t.phase != "" {
		detail = t.phase
	}
	if t.status == taskDone {
		detail = "done"
	}
	parts = append(parts, th.Dim.Render(truncate(detail, max(w-used, 0))))
	return icon + " " + strings.Join(parts, " ")
}

// Progress bars are black squares (U+25A0) for what is done and middle dots
// (U+00B7) for the rest, so they read without color: [■■■■······].
// The brackets count toward the width.
const (
	barDone = "■"
	barTodo = "·"
)

// barCells is how many of w cells are done at pct percent.
func barCells(pct float64, w int) int {
	return int(math.Round(clampF(pct, 0, 100) / 100 * float64(w)))
}

// bar draws a task's progress bar, w cells wide with its brackets. When the
// progress cannot be told, a short run of lit squares slides back and forth instead.
func (m *Model) bar(t *task, w int) string {
	on, off := m.th.ActiveBorder, m.th.Dim
	if t.status == taskDone {
		on = m.th.Success
	}
	in := max(w-2, 1)
	var body string
	if t.indeterminate() && t.status != taskDone {
		seg := max(in/4, 2)
		span := max(in-seg, 0)
		pos := m.spin % max(2*span, 1)
		if pos > span {
			pos = 2*span - pos
		}
		body = barRun(off, barTodo, pos) + barRun(on, barDone, seg) + barRun(off, barTodo, in-seg-pos)
	} else {
		pct := t.pct
		if t.status == taskDone {
			pct = 100
		}
		n := barCells(pct, in)
		body = barRun(on, barDone, n) + barRun(off, barTodo, in-n)
	}
	return m.th.BarEdge.Render("[") + body + m.th.BarEdge.Render("]")
}

// barRun is n cells of one bar character in a style.
func barRun(st lipgloss.Style, ch string, n int) string {
	if n <= 0 {
		return ""
	}
	return st.Render(strings.Repeat(ch, n))
}

func clampF(v, lo, hi float64) float64 { return math.Min(math.Max(v, lo), hi) }

// ---- quick CLI calls --------------------------------------------------------------

// needCLI reports whether the Unity CLI is there to use. When it is not, it
// tells the user that what they asked for needs it.
func (m *Model) needCLI() bool {
	if m.cli == nil {
		m.showCLIMissing()
		return false
	}
	return true
}

// withCLI runs f with the Unity CLI, for something the user asked for; without
// a CLI, the user is told it is needed. Flows that use the CLI go through it,
// and their commands use the client it hands them.
func (m *Model) withCLI(f func(c unityCLI) tea.Cmd) tea.Cmd {
	if !m.needCLI() {
		return nil
	}
	return f(m.cli)
}

// cliCmd is a call to the CLI that lazyunity makes by itself, such as a
// reload. Without a CLI it quietly does nothing.
func (m *Model) cliCmd(f func(ctx context.Context, c unityCLI) tea.Msg) tea.Cmd {
	c := m.cli
	if c == nil {
		return nil
	}
	return func() tea.Msg { return f(context.Background(), c) }
}

// runCLI runs a short CLI call in the background. On success ok is shown and
// after is run; on failure an error popup opens.
func (m *Model) runCLI(what, ok string, f func(ctx context.Context, c unityCLI) error, after func(m *Model) tea.Cmd) tea.Cmd {
	return m.withCLI(func(c unityCLI) tea.Cmd {
		return func() tea.Msg {
			return cliDoneMsg{what: what, ok: ok, err: f(context.Background(), c), after: after}
		}
	})
}

func (m *Model) onCLIDone(msg cliDoneMsg) tea.Cmd {
	if msg.err != nil {
		m.showError(msg.what, msg.err)
		return nil
	}
	if msg.ok != "" {
		m.toastf(toastOK, "%s", msg.ok)
	}
	if msg.after != nil {
		return msg.after(m)
	}
	return nil
}

// showError tells the user what went wrong, with what the CLI printed.
func (m *Model) showError(what string, err error) {
	var e *unitycli.Error
	if errors.As(err, &e) {
		switch {
		case e.NotSignedIn():
			m.push(&confirmPopup{
				title: "Sign in to Unity", yes: "Sign in",
				message: what + " needs you to be signed in to your Unity account.",
				onYes:   func(m *Model) tea.Cmd { return m.signInCmd() },
			})
			return
		case e.InstallLocked():
			m.push(&infoPopup{title: "Another install is running", lines: wrapText(
				what+" could not start: another install is in progress. Unity's tools take turns, so wait for it "+
					"to finish (the official Unity Hub may be installing something) and try again.", 70)})
			return
		}
	}
	lines := wrapText(err.Error(), 76)
	if e != nil && e.Output != "" && e.Output != err.Error() {
		lines = append(lines, "", "Output:")
		for _, l := range strings.Split(e.Output, "\n") {
			lines = append(lines, wrapText(l, 76)...)
		}
	}
	m.push(&infoPopup{title: what + " failed", lines: lines, danger: true})
}

// showCLIMissing explains that the Unity CLI is needed.
func (m *Model) showCLIMissing() {
	m.push(&infoPopup{title: "Unity CLI needed", lines: wrapText(
		"lazyunity does its work through the Unity CLI, and it is not installed or too old.\n\n"+
			"Open the Tools tab of the Options panel (3, then ]) and press enter on Unity CLI to install it.", 70)})
}
