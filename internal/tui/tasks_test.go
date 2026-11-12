package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// quick returns a task that finishes at once with err.
func quick(title string, err error, after func(*Model, error) tea.Cmd) *task {
	return &task{title: title, after: after, do: func(context.Context) error { return err }}
}

func TestTaskRunsAndLingers(t *testing.T) {
	m := newTestModel(t)
	var afterErr error
	called := 0
	cmd := m.enqueue(quick("Doing a thing", nil, func(m *Model, err error) tea.Cmd { called++; afterErr = err; return nil }))
	if m.tasksHeight() != 3 || m.geo.tasks.empty() {
		t.Fatalf("a task needs its frame: height=%d geo=%+v", m.tasksHeight(), m.geo.tasks)
	}
	drain(m, cmd)
	if called != 1 || afterErr != nil {
		t.Fatalf("after: called=%d err=%v", called, afterErr)
	}
	out := screenText(m)
	if !strings.Contains(out, "Doing a thing") || !strings.Contains(out, "✓") || !strings.Contains(out, "done") {
		t.Errorf("a finished task shows a check mark for a moment:\n%s", out)
	}
	// after the linger time the frame goes away and the panels get their space back
	m.now = func() time.Time { return testNow.Add(3 * time.Second) }
	m.Update(spinMsg{})
	if len(m.tasks) != 0 || m.tasksHeight() != 0 || !m.geo.tasks.empty() {
		t.Errorf("the finished task should be gone: tasks=%d", len(m.tasks))
	}
	if m.geo.panels[panelProjects].h != 35 {
		t.Errorf("the panels should use the whole screen again, projects is %d tall", m.geo.panels[panelProjects].h)
	}
}

// background runs a command the way Bubble Tea does: every command, and every
// command inside a batch, on its own goroutine. The messages that come out
// (clock ticks aside) are delivered to the caller, who feeds them to Update, so
// that Update itself only ever runs on the test's goroutine.
func background(cmd tea.Cmd) <-chan tea.Msg {
	out := make(chan tea.Msg, 16)
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		go func() {
			msg := c()
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, sub := range batch {
					run(sub)
				}
				return
			}
			switch msg.(type) {
			case spinMsg, tickMsg, nil:
				return
			}
			out <- msg
		}()
	}
	run(cmd)
	return out
}

func TestTasksRunOneAtATime(t *testing.T) {
	m := newTestModel(t)
	var order []string
	release := make(chan struct{})
	first := &task{title: "first", do: func(context.Context) error { <-release; order = append(order, "first"); return nil }}
	second := quick("second", nil, func(*Model, error) tea.Cmd { order = append(order, "second"); return nil })

	c1 := m.enqueue(first)
	m.enqueue(second)
	if first.status != taskRunning || second.status != taskQueued {
		t.Fatalf("statuses: %v %v", first.status, second.status)
	}
	if !strings.Contains(screenText(m), "queued") {
		t.Error("the waiting task should say so")
	}
	if m.tasksHeight() != 4 {
		t.Errorf("two tasks need two lines, height = %d", m.tasksHeight())
	}

	msgs := background(c1) // runs the first task, which blocks on release
	time.Sleep(20 * time.Millisecond)
	if second.status != taskQueued {
		t.Error("the second task must wait for the first")
	}
	close(release)
	_, next := m.Update(<-msgs) // the first task ends; the second starts
	if second.status != taskRunning {
		t.Errorf("the second task should start when the first ends: %v", second.status)
	}
	drain(m, next)
	if first.status != taskDone || second.status != taskDone || strings.Join(order, ",") != "first,second" {
		t.Errorf("statuses %v %v, order %v", first.status, second.status, order)
	}
}

func TestTaskFailureOpensAnErrorPopup(t *testing.T) {
	m := newTestModel(t)
	var got error
	boom := &unitycli.Error{Args: []string{"install"}, ExitCode: 7, Message: "Could not reach Unity", Output: "dial tcp: timeout"}
	drain(m, m.enqueue(quick("Installing 6000.7.0b8", boom, func(_ *Model, err error) tea.Cmd { got = err; return nil })))
	if got != boom {
		t.Errorf("after received %v", got)
	}
	if len(m.tasks) != 0 {
		t.Error("a failed task leaves the queue")
	}
	if len(m.popups) != 1 {
		t.Fatalf("popups = %d", len(m.popups))
	}
	out := screenText(m)
	for _, want := range []string{"Installing 6000.7.0b8 failed", "Could not reach Unity", "dial tcp: timeout"} {
		if !strings.Contains(out, want) {
			t.Errorf("the error popup should contain %q:\n%s", want, out)
		}
	}
}

func TestCancelledTaskIsNotAnError(t *testing.T) {
	m := newTestModel(t)
	started := make(chan struct{})
	tk := &task{title: "Long job", do: func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() }}
	msgs := background(m.enqueue(tk))
	<-started
	m.cancelTasks()
	_, next := m.Update(<-msgs)
	drain(m, next)
	if len(m.popups) != 0 {
		t.Errorf("cancelling is not an error, but a popup opened")
	}
	if len(m.tasks) != 0 {
		t.Errorf("tasks = %d", len(m.tasks))
	}
	if !strings.Contains(screenText(m), "Cancelled: Long job") {
		t.Error("the user should be told it was cancelled")
	}
	// the CLI reports being interrupted as an exit code
	if !isCancelled(&unitycli.Error{ExitCode: 130}) || !isCancelled(&unitycli.Error{ExitCode: 143}) || isCancelled(errors.New("x")) {
		t.Error("isCancelled")
	}
}

// A stopped process does not always say it was interrupted: on Windows it is
// killed (exit code 1), and a program run with exec.CommandContext reports
// "signal: killed". Whatever the error, a task the user cancelled is cancelled.
func TestCancelledTaskFailingAnyWayIsNotAnError(t *testing.T) {
	for _, err := range []error{&unitycli.Error{ExitCode: 1}, errors.New("openupm add x: signal: killed")} {
		m := newTestModel(t)
		started := make(chan struct{})
		tk := &task{title: "Long job", do: func(ctx context.Context) error { close(started); <-ctx.Done(); return err }}
		msgs := background(m.enqueue(tk))
		<-started
		press(m, "X")
		_, next := m.Update(<-msgs)
		drain(m, next)
		if len(m.popups) != 0 {
			t.Errorf("%v: a cancelled task opened a popup:\n%s", err, screenText(m))
		}
		if !strings.Contains(screenText(m), "Cancelled: Long job") {
			t.Errorf("%v: the user should be told it was cancelled", err)
		}
	}
}

func TestCancelKeyKeepsTheQueue(t *testing.T) {
	m := newTestModel(t)
	started := make(chan struct{})
	msgs := background(m.enqueue(&task{title: "a", do: func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() }}))
	m.enqueue(&task{title: "b", do: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }})
	m.enqueue(quick("c", nil, nil))
	<-started
	press(m, "X")
	if len(m.tasks) != 3 {
		t.Fatalf("X leaves the queued tasks alone: %d tasks", len(m.tasks))
	}
	m.Update(<-msgs) // a ends, cancelled
	if len(m.tasks) != 2 || m.tasks[0].title != "b" || m.tasks[0].status != taskRunning || m.tasks[1].status != taskQueued {
		t.Errorf("the next task starts once the cancelled one ends: %d tasks, first %q", len(m.tasks), m.tasks[0].title)
	}
	m.cancelTasks()
}

func TestCancellingDropsQueuedTasks(t *testing.T) {
	m := newTestModel(t)
	m.enqueue(&task{title: "a", do: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }})
	m.enqueue(quick("b", nil, nil))
	m.enqueue(quick("c", nil, nil))
	m.cancelTasks()
	if len(m.tasks) != 1 || m.tasks[0].title != "a" {
		t.Errorf("only the running task stays until it ends: %d tasks", len(m.tasks))
	}
}

func TestProgressFramesShow(t *testing.T) {
	m := newTestModel(t)
	tk := &task{title: "Installing 6000.7.0b8", do: func(context.Context) error { select {} }}
	m.enqueue(tk)
	pct := func(v float64) *float64 { return &v }

	m.Update(taskFrameMsg{id: tk.id, frame: unitycli.Frame{Type: "progress", Pct: pct(42), Phase: "download", Message: "Downloading Web Build Support..."}})
	out := screenText(m)
	for _, want := range []string{"─Tasks─", "Installing 6000.7.0b8", " 42%", "download · Downloading Web Build Support...", "[■■■■"} {
		if !strings.Contains(out, want) {
			t.Errorf("the task frame should contain %q:\n%s", want, out)
		}
	}
	m.Update(taskFrameMsg{id: tk.id, frame: unitycli.Frame{Type: "progress", Pct: pct(50), Phase: "install", Message: "Installing..."}})
	if !strings.Contains(screenText(m), "install · Installing...") {
		t.Error("the next phase replaces the first")
	}
	// the CLI holds the install phase at 50%: that is not progress worth a number
	if strings.Contains(screenText(m), "50%") || !tk.indeterminate() {
		t.Errorf("the install phase has no percentage:\n%s", screenText(m))
	}
	// the result frame is not progress
	before := tk.pct
	m.Update(taskFrameMsg{id: tk.id, frame: unitycli.Frame{Type: "result"}})
	if tk.pct != before {
		t.Error("the result frame must not change the progress")
	}
	// a frame for a task that is gone is ignored
	m.Update(taskFrameMsg{id: 999, frame: unitycli.Frame{Type: "progress", Pct: pct(1)}})
}

func TestBars(t *testing.T) {
	m := newTestModel(t)
	for _, c := range []struct {
		pct  float64
		done int
	}{{0, 0}, {50, 5}, {100, 10}, {150, 10}, {33, 3}, {36, 4}, {-5, 0}} {
		if got := barCells(c.pct, 10); got != c.done {
			t.Errorf("barCells(%v) = %d; want %d", c.pct, got, c.done)
		}
		if c.pct < 0 {
			continue
		}
		got := stripANSI(m.bar(&task{pct: c.pct, status: taskRunning}, 12))
		want := "[" + strings.Repeat(barDone, c.done) + strings.Repeat(barTodo, 10-c.done) + "]"
		if got != want {
			t.Errorf("bar(%v) = %q, want %q", c.pct, got, want)
		}
	}
	// unknown progress: a run of squares that moves, always the full width
	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		m.spin = i
		b := stripANSI(m.bar(&task{pct: -1, status: taskRunning}, 20))
		inner := strings.TrimSuffix(strings.TrimPrefix(b, "["), "]")
		if width(b) != 20 || utf8.RuneCountInString(inner) != 18 || strings.Trim(inner, barDone+barTodo) != "" || !strings.Contains(inner, barDone) {
			t.Errorf("sliding bar = %q", b)
		}
		seen[b] = true
	}
	if len(seen) < 5 {
		t.Errorf("the segment does not move: %d positions", len(seen))
	}
}

func TestTheSameWorkIsNotQueuedTwice(t *testing.T) {
	m := newTestModel(t)
	block := func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
	first := &task{title: "Adding Android to Unity 6000.6.8f1", works: []string{workModule("6000.6.8f1", "android")}, do: block}
	m.enqueue(first)
	if cmd := m.enqueue(&task{title: "again", works: []string{workModule("6000.6.8f1", "android")}, do: block}); cmd != nil || len(m.tasks) != 1 {
		t.Fatalf("a second install of the same module is refused: %d tasks", len(m.tasks))
	}
	if !strings.Contains(m.toast.text, "Already in the queue") {
		t.Errorf("toast = %q", m.toast.text)
	}
	// another module of the same Editor is fine
	m.enqueue(&task{title: "web", works: []string{workModule("6000.6.8f1", "webgl")}, do: block})
	if len(m.tasks) != 2 {
		t.Errorf("another module queues: %d tasks", len(m.tasks))
	}
	// installModules leaves out what is already queued
	useFakeCLI(t, m) // never run: the first task holds the queue
	ed := editorRow{}
	ed.Version = "6000.6.8f1"
	if cmd := m.installModules(ed, []unitycli.Module{{ID: "android", Name: "Android Build Support"}}); cmd != nil {
		t.Error("nothing new to install: no task")
	}
	m.installModules(ed, []unitycli.Module{{ID: "android", Name: "Android Build Support"}, {ID: "ios", Name: "iOS Build Support"}})
	if len(m.tasks) != 3 || m.tasks[2].label != "6000.6.8f1: iOS Build Support" {
		t.Errorf("only iOS is added: %d tasks, last label %q", len(m.tasks), m.tasks[len(m.tasks)-1].label)
	}
	m.cancelTasks()
}

func TestSeveralTasksShowTheirTitles(t *testing.T) {
	m := newTestModel(t)
	for _, n := range []string{"alpha", "beta", "gamma", "delta", "epsilon"} {
		m.enqueue(&task{title: "Install " + n, do: func(ctx context.Context) error { <-ctx.Done(); return nil }})
	}
	if m.tasksHeight() != maxTaskLines+2 {
		t.Errorf("the frame is capped at %d lines, height = %d", maxTaskLines, m.tasksHeight())
	}
	out := screenText(m)
	for _, want := range []string{"Tasks", "Install alpha", "Install beta", "Install gamma", "+2 more"} {
		if !strings.Contains(out, want) {
			t.Errorf("should contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Install delta") {
		t.Error("only the first tasks fit")
	}
	for i, l := range screen(m) {
		if width(l) != 120 {
			t.Errorf("line %d is %d wide", i, width(l))
		}
	}
	m.cancelTasks()
}

func TestTasksFrameTakesSpaceFromThePanels(t *testing.T) {
	m := newTestModel(t)
	before := m.geo.panels[panelProjects].h
	m.enqueue(&task{title: "x", do: func(ctx context.Context) error { <-ctx.Done(); return nil }})
	if got := m.geo.panels[panelProjects].h; got != before-3 {
		t.Errorf("projects panel went from %d to %d, want %d", before, got, before-3)
	}
	covered(t, m.geo, 120, 36-1-3)
	m.cancelTasks()
}

func TestQuitWhileBusyAsks(t *testing.T) {
	m := newTestModel(t)
	cancelled := false
	m.enqueue(&task{title: "Installing", do: func(ctx context.Context) error { <-ctx.Done(); cancelled = true; return ctx.Err() }})
	press(m, "q")
	if m.quit || len(m.popups) != 1 {
		t.Fatalf("quitting during a task must ask first: quit=%v popups=%d", m.quit, len(m.popups))
	}
	if !strings.Contains(screenText(m), "A task is still running") {
		t.Error("the question should say why")
	}
	press(m, "esc")
	if m.quit || len(m.popups) != 0 {
		t.Error("esc keeps running")
	}
	press(m, "q", "enter")
	if !m.quit {
		t.Error("confirming should quit")
	}
	if tk := m.running(); tk == nil || tk.cancel == nil {
		t.Fatal("running task lost")
	}
	_ = cancelled
}

func TestCtrlCDuringPopupAlsoAsksWhenBusy(t *testing.T) {
	m := newTestModel(t)
	m.enqueue(&task{title: "Installing", do: func(ctx context.Context) error { <-ctx.Done(); return nil }})
	m.push(&infoPopup{title: "x", lines: []string{"y"}})
	press(m, "ctrl+c")
	if m.quit {
		t.Error("ctrl+c while a task runs must ask")
	}
	m.cancelTasks()
}

func TestRunCLI(t *testing.T) {
	m := newTestModel(t)
	useFakeCLI(t, m) // only its presence matters: f below does not use it
	ranAfter := 0
	cmd := m.runCLI("Pinning", "Pinned", func(context.Context, unityCLI) error { return nil },
		func(*Model) tea.Cmd { ranAfter++; return nil })
	drain(m, cmd)
	if ranAfter != 1 || !strings.Contains(screenText(m), "Pinned") {
		t.Errorf("success: after=%d", ranAfter)
	}

	cmd = m.runCLI("Pinning", "", func(context.Context, unityCLI) error {
		return &unitycli.Error{ExitCode: 1, Message: "no such project"}
	}, func(*Model) tea.Cmd { ranAfter++; return nil })
	drain(m, cmd)
	if ranAfter != 1 || len(m.popups) != 1 || !strings.Contains(screenText(m), "no such project") {
		t.Errorf("failure: after=%d popups=%d", ranAfter, len(m.popups))
	}

	m.popups = nil
	m.cli = nil
	if cmd := m.runCLI("x", "", nil, nil); cmd != nil || len(m.popups) != 1 || !strings.Contains(screenText(m), "Unity CLI needed") {
		t.Error("without a CLI the user is told how to get one")
	}
}

func TestShowErrorVariants(t *testing.T) {
	m := newTestModel(t)

	m.showError("Opening", &unitycli.Error{ExitCode: unitycli.ExitAuth, Message: "not signed in"})
	if _, ok := m.popups[0].(*confirmPopup); !ok || !strings.Contains(screenText(m), "Sign in") {
		t.Errorf("not signed in should offer to sign in:\n%s", screenText(m))
	}
	m.popups = nil

	m.showError("Installing", &unitycli.Error{ExitCode: unitycli.ExitInstallLocked, Message: "locked"})
	if !strings.Contains(screenText(m), "Another install is running") || !strings.Contains(screenText(m), "official Unity Hub") {
		t.Errorf("locked:\n%s", screenText(m))
	}
	m.popups = nil

	m.showError("Opening", errors.New("plain failure"))
	if !strings.Contains(screenText(m), "Opening failed") || !strings.Contains(screenText(m), "plain failure") {
		t.Errorf("plain:\n%s", screenText(m))
	}
	m.popups = nil

	long := strings.Repeat("a long line of CLI output ", 10)
	m.showError("Opening", &unitycli.Error{ExitCode: 6, Message: "failed", Output: "first\n" + long})
	out := screenText(m)
	if !strings.Contains(out, "Output:") || !strings.Contains(out, "first") {
		t.Errorf("output should follow the message:\n%s", out)
	}
	for i, l := range screen(m) {
		if width(l) != 120 {
			t.Errorf("line %d is %d wide", i, width(l))
		}
	}
}

func TestSignInTask(t *testing.T) {
	m := newTestModel(t)
	m.cli = nil
	if cmd := m.signInCmd(); cmd != nil || len(m.popups) != 1 {
		t.Error("signing in needs the CLI")
	}
}

// enqueueForever starts a task that only ends when cancelled.
func enqueueForever(m *Model) {
	m.enqueue(&task{title: "Forever", do: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }})
}

func TestAQueueShowsShortLabels(t *testing.T) {
	m := newTestModel(t)
	block := func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
	m.enqueue(&task{title: "Installing Unity 6000.3.28f1 + Android Build Support and 3 more", label: "6000.3.28f1", do: block})
	m.enqueue(&task{title: "Adding Windows Build Support (Mono) to Unity 6000.6.8f1", label: "6000.6.8f1: Windows Build Support (Mono)", do: block})
	out := screenText(m)
	for _, want := range []string{"6000.3.28f1 ", "6000.6.8f1: Windows Build Support (Mono)"} {
		if !strings.Contains(out, want) {
			t.Errorf("should contain %q:\n%s", want, out)
		}
	}
	for _, l := range screen(m) {
		if strings.Contains(l, "6000.") && (strings.Contains(l, "Installing Unity") || strings.Contains(l, "…")) {
			t.Errorf("the long titles stay out of a queue, and the labels fit: %q", l)
		}
	}
	m.cancelTasks()
}

// Every flow that needs the CLI says so when there is none, including the ones
// reached from a popup that was opened while there still was one.
func TestFlowsWithoutTheCLIExplainItIsNeeded(t *testing.T) {
	flows := map[string]func(m *Model) tea.Cmd{
		"plan an install":   func(m *Model) tea.Cmd { return m.planInstall(unitycli.InstallOptions{Version: "6000.5.1f1"}) },
		"list templates":    func(m *Model) tea.Cmd { return m.loadTemplatesCmd(newProject{name: "G", version: "6000.6.8f1"}) },
		"install an Editor": func(m *Model) tea.Cmd { return m.installEditorFlow("6000.5.1f1", "") },
		"open a project":    func(m *Model) tea.Cmd { r := m.projectsTab.Items()[0]; return m.runOpen(r, m.planOpen(r)) },
		"pin a project":     func(m *Model) tea.Cmd { return m.togglePin(m.projectsTab.Items()[0]) },
		"create a project": func(m *Model) tea.Cmd {
			return m.createProject(newProject{name: "G", parent: t.TempDir()}, unitycli.CreateOptions{}, "", nil)
		},
		"remove a module": func(m *Model) tea.Cmd {
			m.removeModuleConfirm(m.inv.editors()[0], unitycli.Module{ID: "webgl", Name: "WebGL"})
			m.cli = nil // the CLI went away while the question was open
			_, cmd := m.Update(keyMsg("enter"))
			return cmd
		},
	}
	for name, flow := range flows {
		m := newTestModel(t)
		if name != "remove a module" {
			m.cli = nil
		} else {
			useFakeCLI(t, m)
		}
		drain(m, flow(m))
		if len(m.popups) == 0 || !strings.Contains(screenText(m), "Unity CLI needed") {
			t.Errorf("%s without the CLI should say it is needed:\n%s", name, screenText(m))
		}
		if len(m.tasks) != 0 {
			t.Errorf("%s without the CLI queued a task", name)
		}
	}
}
