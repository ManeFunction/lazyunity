package unitycli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// LogEntry records one CLI invocation for the command log.
type LogEntry struct {
	Time     time.Time
	Args     []string
	Duration time.Duration
	ExitCode int
	Stdout   string // trimmed
	Stderr   string // trimmed
}

// Command renders the invocation as a shell-like string.
func (l LogEntry) Command() string {
	parts := make([]string, 0, len(l.Args)+1)
	parts = append(parts, "unity")
	for _, a := range l.Args {
		if strings.ContainsAny(a, " \t\"'") {
			a = fmt.Sprintf("%q", a)
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// maxLogEntries bounds the in-memory command log.
const maxLogEntries = 200

// Client runs the Unity CLI.
type Client struct {
	// Path is the CLI executable.
	Path string

	// newCmd builds the process to run; tests replace it.
	newCmd func(ctx context.Context, name string, args ...string) *exec.Cmd

	mu   sync.Mutex
	log  []LogEntry
	hook func(LogEntry)
}

// New returns a Client for the CLI at path.
func New(path string) *Client {
	return &Client{Path: path, newCmd: exec.CommandContext}
}

// NewWithCommand returns a Client that builds its processes with newCmd instead
// of running path. It exists so that other packages' tests can stand in a fake
// CLI.
func NewWithCommand(path string, newCmd func(ctx context.Context, name string, args ...string) *exec.Cmd) *Client {
	return &Client{Path: path, newCmd: newCmd}
}

// OnLog registers fn to be called, from any goroutine, for every finished
// invocation.
func (c *Client) OnLog(fn func(LogEntry)) {
	c.mu.Lock()
	c.hook = fn
	c.mu.Unlock()
}

// Log returns a copy of the command log, oldest first.
func (c *Client) Log() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]LogEntry(nil), c.log...)
}

func (c *Client) record(e LogEntry) {
	c.mu.Lock()
	c.log = append(c.log, e)
	if len(c.log) > maxLogEntries {
		c.log = c.log[len(c.log)-maxLogEntries:]
	}
	hook := c.hook
	c.mu.Unlock()
	if hook != nil {
		hook(e)
	}
}

// runOpts tunes one invocation.
type runOpts struct {
	// interactive lets the CLI ask questions and open a browser; the sign-in
	// needs it.
	interactive bool
}

// command builds the process for args. Output format and prompts are set
// through the CLI's documented environment variables rather than flags, so they
// cannot swallow positional arguments or end up inside variadic options.
func (c *Client) command(ctx context.Context, o runOpts, args []string) *exec.Cmd {
	cmd := c.newCmd(ctx, c.Path, args...)
	env := append(os.Environ(),
		"UNITY_FORMAT=json",
		"UNITY_NO_BANNER=1",
		"UNITY_NO_PAGER=1",
		"NO_COLOR=1",
	)
	if !o.interactive {
		env = append(env, "UNITY_NON_INTERACTIVE=1")
	}
	cmd.Env = append(env, cmd.Env...) // keep anything a test added
	return cmd
}

// Run runs the CLI and decodes its JSON envelope. A non-nil envelope may be
// returned together with an error, for commands whose failure carries data.
func (c *Client) Run(ctx context.Context, args ...string) (*Envelope, error) {
	return c.run(ctx, runOpts{}, args)
}

func (c *Client) run(ctx context.Context, o runOpts, args []string) (*Envelope, error) {
	start := time.Now()
	res, err := runToFiles(c.command(ctx, o, args))
	c.record(LogEntry{
		Time:     start,
		Args:     args,
		Duration: time.Since(start),
		ExitCode: res.ExitCode,
		Stdout:   tail(res.Stdout, outputTailLimit),
		Stderr:   tail(res.Stderr, outputTailLimit),
	})
	if err != nil {
		return nil, &Error{Args: args, ExitCode: -1, Message: "could not start the Unity CLI: " + err.Error()}
	}

	env, perr := parseEnvelope(res.Stdout)
	if res.ExitCode == 0 && perr == nil && env.Success {
		return env, nil
	}

	e := &Error{Args: args, ExitCode: res.ExitCode, Output: tail(res.Stderr+"\n"+res.Stdout, outputTailLimit)}
	if env != nil && len(env.Errors) > 0 {
		e.Code = env.Errors[0].Code
		e.Message = env.Errors[0].Message
	}
	if e.Message == "" {
		switch {
		case res.ExitCode == 0 && perr != nil:
			e.Message = fmt.Sprintf("unexpected output from unity %s: %v", strings.Join(args, " "), perr)
		case strings.TrimSpace(res.Stderr) != "":
			e.Message = firstLine(res.Stderr)
		}
	}
	return env, e
}

// parseEnvelope decodes the JSON envelope, tolerating stray text before it.
func parseEnvelope(out string) (*Envelope, error) {
	i := strings.Index(out, "{")
	if i < 0 {
		return nil, fmt.Errorf("no JSON in output")
	}
	var env Envelope
	if err := json.NewDecoder(strings.NewReader(out[i:])).Decode(&env); err != nil {
		return nil, err
	}
	return &env, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
