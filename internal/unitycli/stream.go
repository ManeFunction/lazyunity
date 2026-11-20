package unitycli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Frame is one line of `--format ndjson` output. Installs print progress
// frames and finish with a frame whose Type is "result".
type Frame struct {
	Type string `json:"type"`

	// progress frames
	Phase         string   `json:"phase"`
	Pct           *float64 `json:"pct"`
	UID           string   `json:"uid"`
	ParentItemUID string   `json:"parentItemUid"`
	Name          string   `json:"name"`
	Message       string   `json:"msg"`

	// the result frame
	Success *bool           `json:"success"`
	Command string          `json:"command"`
	Data    json.RawMessage `json:"data"`
	Errors  []APIError      `json:"errors"`

	// Raw is the line as printed, for the command log.
	Raw string `json:"-"`
}

// IsResult reports whether this is the final frame.
func (f Frame) IsResult() bool { return f.Type == "result" }

// Stream is a running ndjson command.
type Stream struct {
	// Frames delivers every frame; it is closed once the process has ended.
	Frames <-chan Frame

	done chan struct{}
	err  error
}

// Wait blocks until the command has ended and returns its error, if any. Call
// it after Frames is closed, or instead of reading Frames: it discards any
// frames still unread so that the command can finish.
func (s *Stream) Wait() error {
	for range s.Frames {
	}
	<-s.done
	return s.err
}

// FinishedStream is a stream that has already ended: frames are still there to
// read, and Wait returns err. It stands in for a running command where a
// Client is replaced, such as in the tests of the packages that use one.
func FinishedStream(err error, frames ...Frame) *Stream {
	ch := make(chan Frame, len(frames))
	for _, f := range frames {
		ch <- f
	}
	close(ch)
	done := make(chan struct{})
	close(done)
	return &Stream{Frames: ch, done: done, err: err}
}

// HeldStream is a stream that reports frames and then stays open, as if its
// command were still running, until ctx is done; Wait then returns ctx's
// error. It stands in for a long install in lazyunity's fake mode.
func HeldStream(ctx context.Context, frames ...Frame) *Stream {
	ch := make(chan Frame, len(frames))
	for _, f := range frames {
		ch <- f
	}
	s := &Stream{Frames: ch, done: make(chan struct{})}
	go func() {
		<-ctx.Done()
		s.err = ctx.Err()
		close(ch)
		close(s.done)
	}()
	return s
}

// pipeGrace is how long to keep reading after the process has exited. The CLI's
// helpers may hold the pipe open forever, so reading to EOF is not an option.
const pipeGrace = 500 * time.Millisecond

// Stream runs a command with --format ndjson and returns its frames as they
// arrive. Cancelling ctx interrupts the command.
func (c *Client) Stream(ctx context.Context, args ...string) (*Stream, error) {
	full := append([]string(nil), args...)
	cmd := c.command(ctx, runOpts{format: "ndjson"}, full)
	prepare(cmd)

	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	errF, err := os.CreateTemp("", "lazyunity-err-*")
	if err != nil {
		r.Close()
		w.Close()
		return nil, err
	}
	cmd.Stdout = w
	cmd.Stderr = errF
	cmd.Stdin = nil

	start := time.Now()
	if err := cmd.Start(); err != nil {
		r.Close()
		w.Close()
		cleanup(errF)
		return nil, &Error{Args: full, ExitCode: -1, Message: "could not start the Unity CLI: " + err.Error()}
	}
	w.Close() // the child has its own copy

	frames := make(chan Frame, 64)
	s := &Stream{Frames: frames, done: make(chan struct{})}

	readDone := make(chan struct{})
	var result *Frame
	var sawResult bool
	var raw strings.Builder
	go func() {
		defer close(readDone)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var f Frame
			if json.Unmarshal([]byte(line), &f) != nil {
				continue // not a frame, e.g. a stray log line
			}
			f.Raw = line
			if raw.Len() < 64*1024 {
				raw.WriteString(line)
				raw.WriteByte('\n')
			}
			if sawResult {
				continue // the result is the last frame the reader needs
			}
			if f.IsResult() {
				fc := f
				result = &fc
				sawResult = true
			}
			queueFrame(frames, f)
		}
	}()

	go func() {
		waitErr := cmd.Wait()
		select {
		case <-readDone:
		case <-time.After(pipeGrace):
		}
		r.Close()
		<-readDone
		close(frames)

		code := exitCodeOf(waitErr)
		stderr := readAll(errF)
		cleanup(errF)

		c.record(LogEntry{
			Time:     start,
			Args:     full,
			Duration: time.Since(start),
			ExitCode: code,
			Stdout:   tail(raw.String(), outputTailLimit),
			Stderr:   tail(stderr, outputTailLimit),
		})
		s.err = streamError(full, code, result, stderr)
		close(s.done)
	}()
	return s, nil
}

// queueFrame queues a frame without ever waiting. The frames are read by
// lazyunity's event loop, which stops while the terminal is handed over (a
// shell command, $EDITOR, ctrl+z). Waiting for it then would stop reading the
// pipe, and once the pipe is full the CLI blocks and the install pauses. So
// when the queue is full, the progress waiting in it is folded, in order, into
// one frame that says where things stand now: only the newest progress is
// shown anyway. Nothing is queued after the result, so it is never folded.
// frames has a single sender, this one, so once drained there is room.
func queueFrame(frames chan Frame, f Frame) {
	select {
	case frames <- f:
		return
	default:
	}
	var acc *Frame
	for drained := false; !drained; {
		select {
		case old := <-frames:
			if acc != nil {
				old = carryOver(*acc, old)
			}
			acc = &old
		default:
			drained = true
		}
	}
	if acc != nil {
		if f.IsResult() {
			frames <- *acc
		} else {
			f = carryOver(*acc, f)
		}
	}
	frames <- f
}

// carryOver fills what newer leaves empty from older, the frame it replaces.
func carryOver(older, newer Frame) Frame {
	if newer.Phase == "" {
		newer.Phase = older.Phase
	}
	if newer.Message == "" {
		newer.Message = older.Message
	}
	if newer.Pct == nil {
		newer.Pct = older.Pct
	}
	return newer
}

// streamError turns the end of a stream into an error, or nil on success.
func streamError(args []string, code int, result *Frame, stderr string) error {
	if code == 0 && result != nil && (result.Success == nil || *result.Success) {
		return nil
	}
	e := &Error{Args: args, ExitCode: code, Output: tail(stderr, outputTailLimit)}
	if result != nil && len(result.Errors) > 0 {
		e.Code = result.Errors[0].Code
		e.Message = result.Errors[0].Message
	}
	if e.Message == "" {
		switch {
		case code == 0:
			e.Message = fmt.Sprintf("unity %s ended without a result", strings.Join(args, " "))
		case strings.TrimSpace(stderr) != "":
			e.Message = firstLine(stderr)
		}
	}
	return e
}
