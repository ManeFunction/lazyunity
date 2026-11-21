package tui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// cliDoneMsg is the end of a quick CLI call started with runCLI.
type cliDoneMsg struct {
	what  string
	err   error
	ok    string
	after func(m *Model) tea.Cmd
}

// needCLI reports whether the Unity CLI is there to use. When it is not, it
// tells the user that what they asked for needs it.
func (m *Model) needCLI() bool {
	if m.safe != nil {
		m.toastf(toastWarn, "Safe mode: bring back a Unity CLI lazyunity is verified with first")
		return false
	}
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
	if c == nil || m.safe != nil {
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
