package unitycli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	// queryTimeout bounds a query: a call that only reads, such as a listing.
	queryTimeout time.Duration

	// elevate runs the CLI again as administrator, for a call the CLI refused
	// because it needs rights it cannot ask for itself without a terminal.
	// Nil where lazyunity cannot do that (everywhere but Windows); tests
	// replace it.
	elevate func(ctx context.Context, path string, args []string) (processResult, error)

	mu  sync.Mutex
	log []LogEntry
}

// New returns a Client for the CLI at path.
func New(path string) *Client {
	return &Client{Path: path, newCmd: exec.CommandContext, queryTimeout: QueryTimeout, elevate: runElevated}
}

// QueryTimeout is how long a query may take before it is given up on. It is
// generous: some queries go over the network. Calls that change things, or
// that the user waits on by choice (an install, unity open, the sign-in), have
// no deadline of their own.
const QueryTimeout = time.Minute

// Log returns a copy of the command log, oldest first.
func (c *Client) Log() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]LogEntry(nil), c.log...)
}

func (c *Client) record(e LogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = append(c.log, e)
	if len(c.log) > maxLogEntries {
		c.log = c.log[len(c.log)-maxLogEntries:]
	}
}

// runOpts tunes one invocation.
type runOpts struct {
	// interactive lets the CLI ask questions and open a browser; the sign-in
	// needs it.
	interactive bool

	// format is the output format; empty means json.
	format string
}

// globalFlags are the CLI's global options for o. They go before the command,
// where they cannot swallow positional arguments or end up inside variadic
// options. They are flags rather than the equivalent UNITY_* and NO_COLOR
// environment variables because the CLI's children inherit its environment:
// an Editor started by `unity open` would pass them on to every terminal, IDE
// and `unity` command started from it.
func globalFlags(o runOpts) []string {
	format := o.format
	if format == "" {
		format = "json"
	}
	flags := []string{"--format", format, "--no-banner", "--no-pager", "--no-color"}
	if !o.interactive {
		flags = append(flags, "--non-interactive")
	}
	return flags
}

// command builds the process for args.
func (c *Client) command(ctx context.Context, o runOpts, args []string) *exec.Cmd {
	return c.newCmd(ctx, c.Path, append(globalFlags(o), args...)...)
}

// Run runs the CLI and decodes its JSON envelope. A non-nil envelope may be
// returned together with an error, for commands whose failure carries data.
//
// When the CLI refuses a change because it needs administrator rights (an
// Editor installed in Program Files, say) and lazyunity can get them, the call
// is made again as administrator, which brings up the system's prompt.
func (c *Client) Run(ctx context.Context, args ...string) (*Envelope, error) {
	env, err := c.run(ctx, runOpts{}, args)
	var e *Error
	if c.elevate != nil && errors.As(err, &e) && e.NeedsElevation() {
		return c.runElevated(ctx, args)
	}
	return env, err
}

// runElevated runs the CLI as administrator.
func (c *Client) runElevated(ctx context.Context, args []string) (*Envelope, error) {
	start := time.Now()
	res, err := c.elevate(ctx, c.Path, append(globalFlags(runOpts{}), args...))
	if errors.Is(err, errElevationDeclined) {
		c.record(LogEntry{Time: start, Args: args, Duration: time.Since(start), ExitCode: -1, Stderr: "(as administrator) " + err.Error()})
		return nil, &Error{Args: args, ExitCode: -1, Code: CodeElevationDeclined,
			Message: "This needs administrator rights, and the prompt for them was declined. Nothing was changed."}
	}
	return c.finish(args, start, res, err, "could not run the Unity CLI as administrator: ")
}

// query is Run for a call that only reads, bounded by the query timeout so a
// CLI that never answers cannot leave its caller waiting forever.
func (c *Client) query(ctx context.Context, args ...string) (*Envelope, error) {
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()
	env, err := c.run(ctx, runOpts{}, args)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return env, &Error{Args: args, ExitCode: ExitTerminated,
			Message: fmt.Sprintf("unity %s did not answer within %s", strings.Join(args, " "), c.queryTimeout)}
	}
	return env, err
}

func (c *Client) run(ctx context.Context, o runOpts, args []string) (*Envelope, error) {
	start := time.Now()
	res, err := runToFiles(c.command(ctx, o, args))
	return c.finish(args, start, res, err, "could not start the Unity CLI: ")
}

// finish logs a finished CLI process and decodes what it printed. startErr is
// the error from starting it, reported after prefix.
func (c *Client) finish(args []string, start time.Time, res processResult, startErr error, prefix string) (*Envelope, error) {
	c.record(LogEntry{
		Time:     start,
		Args:     args,
		Duration: time.Since(start),
		ExitCode: res.ExitCode,
		Stdout:   tail(res.Stdout, outputTailLimit),
		Stderr:   tail(res.Stderr, outputTailLimit),
	})
	if startErr != nil {
		return nil, &Error{Args: args, ExitCode: -1, Message: prefix + startErr.Error()}
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
