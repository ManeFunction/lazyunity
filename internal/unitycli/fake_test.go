package unitycli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeResp is what the fake CLI does for one argument list.
type fakeResp struct {
	File   string   `json:"file,omitempty"`   // print this fixture from testdata/cli
	Stdout string   `json:"stdout,omitempty"` // or this text
	Lines  []string `json:"lines,omitempty"`  // or these lines (ndjson)
	Stderr string   `json:"stderr,omitempty"`
	Exit   int      `json:"exit,omitempty"`
	Sleep  string   `json:"sleep,omitempty"` // wait this long first, e.g. "30s"
}

// fakeClient returns a Client whose CLI is the test binary playing the role of
// `unity`. table maps the space-joined arguments to a response; an argument list
// that is not in the table makes the fake exit with 99.
func fakeClient(t *testing.T, table map[string]fakeResp) *Client {
	t.Helper()
	data, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}
	c := New("unity")
	c.newCmd = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "FAKE_UNITY_TABLE="+string(data),
			"GORACE=atexit_sleep_ms=0") // a race build otherwise sleeps 1s before every exit
		return cmd
	}
	return c
}

// TestHelperProcess is the fake `unity`. It is not a test.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	key := strings.Join(args, " ")

	// "env-probe" reports how the CLI was configured.
	if key == "env-probe" {
		fmt.Printf(`{"success":true,"command":"env-probe","data":{"format":%q,"nonInteractive":%q,"banner":%q}}`,
			os.Getenv("UNITY_FORMAT"), os.Getenv("UNITY_NON_INTERACTIVE"), os.Getenv("UNITY_NO_BANNER"))
		os.Exit(0)
	}

	var table map[string]fakeResp
	if err := json.Unmarshal([]byte(os.Getenv("FAKE_UNITY_TABLE")), &table); err != nil {
		fmt.Fprintln(os.Stderr, "bad table:", err)
		os.Exit(98)
	}
	resp, ok := table[key]
	if !ok {
		fmt.Fprintf(os.Stderr, "fake unity: unexpected arguments: %q\n", key)
		os.Exit(99)
	}
	if resp.Sleep != "" {
		d, _ := time.ParseDuration(resp.Sleep)
		time.Sleep(d)
	}
	switch {
	case resp.File != "":
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "cli", resp.File))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(97)
		}
		os.Stdout.Write(b)
	case len(resp.Lines) > 0:
		for _, l := range resp.Lines {
			fmt.Println(l)
		}
	default:
		fmt.Print(resp.Stdout)
	}
	if resp.Stderr != "" {
		fmt.Fprint(os.Stderr, resp.Stderr)
	}
	os.Exit(resp.Exit)
}
