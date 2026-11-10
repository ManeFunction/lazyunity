// Package unitycli is the only way lazyunity reads or changes Unity state.
// It wraps the official `unity` command line tool and speaks its JSON output.
package unitycli

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Envelope is the wrapper every `unity --json` command prints.
type Envelope struct {
	Success       bool              `json:"success"`
	Command       string            `json:"command"`
	Data          json.RawMessage   `json:"data"`
	Errors        []APIError        `json:"errors"`
	Warnings      []json.RawMessage `json:"warnings"`
	Notifications []Notification    `json:"notifications"`
}

// APIError is one entry of Envelope.Errors.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Notification is an informational message such as "a CLI update is available".
type Notification struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Exit codes documented by the Unity CLI.
const (
	ExitOK            = 0
	ExitError         = 1
	ExitBadArgs       = 2
	ExitAuth          = 3
	ExitNeedsConfig   = 4
	ExitCommandFailed = 6
	ExitNetwork       = 7
	ExitInstallLocked = 9
	ExitNotFound      = 11
	ExitInterrupted   = 130
	ExitTerminated    = 143
)

// Error is a failed CLI invocation.
type Error struct {
	Args     []string // the arguments passed after the program name
	ExitCode int      // process exit code, -1 if the process could not be started
	Code     string   // errors[0].code from the JSON envelope, if there was one
	Message  string   // errors[0].message, or a short description
	Output   string   // tail of the output, for display
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("unity %s: exit code %d", strings.Join(e.Args, " "), e.ExitCode)
}

// NotSignedIn reports that the CLI needs a sign-in (exit code 3).
func (e *Error) NotSignedIn() bool { return e.ExitCode == ExitAuth }

// NeedsConfig reports that something such as a license is missing (exit code 4).
func (e *Error) NeedsConfig() bool { return e.ExitCode == ExitNeedsConfig }

// Network reports an unreachable Unity service (exit code 7).
func (e *Error) Network() bool { return e.ExitCode == ExitNetwork }

// InstallLocked reports that another install holds the lock (exit code 9).
func (e *Error) InstallLocked() bool { return e.ExitCode == ExitInstallLocked }

// NotFound reports that a project, release or module does not exist (exit code 11).
func (e *Error) NotFound() bool { return e.ExitCode == ExitNotFound }

// Cancelled reports that the process was interrupted or terminated.
func (e *Error) Cancelled() bool {
	return e.ExitCode == ExitInterrupted || e.ExitCode == ExitTerminated
}

// outputTailLimit is how much of a failed command's output is kept for display.
const outputTailLimit = 2000

// tail returns the last n bytes of s, cut on a rune boundary, prefixed with an
// ellipsis when something was cut.
func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	s = s[len(s)-n:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return "…" + s
}

// decodeData unmarshals the envelope's data into v.
func (e *Envelope) decodeData(v any) error {
	if len(e.Data) == 0 || string(e.Data) == "null" {
		return nil
	}
	return json.Unmarshal(e.Data, v)
}

// items decodes data.items, the shape every list command uses.
func decodeItems[T any](e *Envelope) ([]T, error) {
	var d struct {
		Items []T `json:"items"`
	}
	if err := e.decodeData(&d); err != nil {
		return nil, fmt.Errorf("decoding %q output: %w", e.Command, err)
	}
	return d.Items, nil
}
