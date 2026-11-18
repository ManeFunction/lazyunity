package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

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
	works []string
	// removing marks a task that removes what it works on, such as a module,
	// so its rows show a cross instead of a download arrow.
	removing bool
	// keep marks a task that must not be stopped halfway, such as an
	// uninstall: the CLI deletes files as it goes, so a stopped one leaves a
	// broken Editor behind. It cannot be cancelled, and lazyunity will not
	// quit or restart while it runs.
	keep   bool
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

// ---- the queue -----------------------------------------------------------------

const (
	// spinInterval is one frame of the progress scanner; the spinner icon
	// steps every spinnerStep, whatever the scanner's pace.
	spinInterval  = 45 * time.Millisecond
	spinnerStep   = 60 * time.Millisecond
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

// workCLI is the Unity CLI itself, while it updates or changes feed.
const workCLI = "cli"

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
	step := m.spin * int(spinInterval) / int(spinnerStep)
	return string(frames[step%len(frames)])
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
		ctx := context.Background()
		if !t.keep {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			t.cancel = cancel
		}
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

// confirmCancelRunning asks before stopping the running task. The queued ones
// stay: the next starts once the running one has ended. Only the task
// asked about is stopped: if it ends while the question is open, the answer
// does not reach the one that started after it.
func (m *Model) confirmCancelRunning() {
	t := m.running()
	if t == nil {
		return
	}
	if t.keep {
		m.toastf(toastWarn, "%s cannot be cancelled: stopping it halfway breaks what it works on", t.title)
		return
	}
	m.push(&confirmPopup{
		title: "Cancel task", yes: "Cancel task", no: "Keep running", danger: true,
		message: fmt.Sprintf("Cancel %q?", t.title),
		onYes: func(m *Model) tea.Cmd {
			if m.running() == t {
				t.stop()
			}
			return nil
		},
	})
}

// stop cancels a running task, unless it must not be stopped.
func (t *task) stop() {
	if t.cancel != nil && !t.keep {
		t.cancelled = true
		t.cancel()
	}
}

// cancellable reports whether a task is running that the user may stop.
func (m *Model) cancellable() bool {
	t := m.running()
	return t != nil && !t.keep
}

// refuseWhileKept tells the user to wait, and reports true, while a task that
// cannot be stopped is running, so quitting or restarting does not kill it.
func (m *Model) refuseWhileKept(what string) bool {
	t := m.running()
	if t == nil || !t.keep {
		return false
	}
	m.toastf(toastWarn, "Wait for %q to finish before %s: stopping it halfway breaks what it works on", t.title, what)
	return true
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
	msg := trimEllipsis(t.msg)
	detail := msg
	if t.phase != "" && msg != "" {
		detail = t.phase + " · " + msg
	} else if t.phase != "" {
		detail = t.phase
	}
	if t.status == taskDone {
		detail = "done"
	}
	parts = append(parts, th.Dim.Render(truncate(detail, max(w-used, 0))))
	return icon + " " + strings.Join(parts, " ")
}

// trimEllipsis drops the trailing "..." the CLI puts on its progress messages
// ("Downloading 6000.6.9f1-x86_64..."): the spinner already says the task is
// going, and many fonts draw "..." as one glyph that reads like the … a cut
// line ends in, so a whole message looked truncated.
func trimEllipsis(s string) string {
	s = strings.TrimSpace(s)
	for {
		t := strings.TrimSuffix(strings.TrimSuffix(s, "..."), "…")
		if t == s {
			return strings.TrimSpace(s)
		}
		s = t
	}
}
