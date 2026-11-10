package unitycli

import (
	"errors"
	"os"
	"os/exec"
	"time"
)

// processResult is what a finished CLI process left behind.
type processResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// interruptGrace is how long a cancelled CLI gets to stop on its own before it
// is killed.
const interruptGrace = 5 * time.Second

// prepare detaches cmd from lazyunity's terminal session and makes context
// cancellation interrupt it politely instead of killing it outright.
func prepare(cmd *exec.Cmd) {
	setSysProcAttr(cmd)
	cmd.Cancel = func() error { return interruptProcess(cmd) }
	cmd.WaitDelay = interruptGrace
}

// runToFiles runs cmd to completion with its output going to temporary files.
//
// Files rather than pipes, on purpose: `unity open` leaves helper processes
// (the Editor, the sign-in service) running after the CLI itself exits, and
// they inherit its output. With a pipe, waiting for the CLI would block until
// those helpers exit, and closing the pipe would kill them with EPIPE. A file
// has neither problem.
func runToFiles(cmd *exec.Cmd) (processResult, error) {
	outF, err := os.CreateTemp("", "lazyunity-out-*")
	if err != nil {
		return processResult{}, err
	}
	errF, err := os.CreateTemp("", "lazyunity-err-*")
	if err != nil {
		cleanup(outF)
		return processResult{}, err
	}
	defer cleanup(outF)
	defer cleanup(errF)

	cmd.Stdout = outF
	cmd.Stderr = errF
	cmd.Stdin = nil
	prepare(cmd)

	if err := cmd.Start(); err != nil {
		return processResult{ExitCode: -1}, err
	}
	waitErr := cmd.Wait()

	res := processResult{ExitCode: exitCodeOf(waitErr)}
	res.Stdout = readAll(outF)
	res.Stderr = readAll(errF)
	return res, nil
}

// exitCodeOf converts the error from Wait into an exit code.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
		return ExitTerminated // killed by a signal
	}
	return -1
}

func readAll(f *os.File) string {
	b, err := os.ReadFile(f.Name())
	if err != nil {
		return ""
	}
	return string(b)
}

func cleanup(f *os.File) {
	f.Close()
	os.Remove(f.Name())
}
