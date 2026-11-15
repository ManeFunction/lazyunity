package unitycli

import (
	"bufio"
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
	Repeat int      `json:"repeat,omitempty"` // print the first of Lines this many times
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
	// The global options come first; the table is keyed by what follows them.
	var globals []string
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		n := 1
		if args[0] == "--format" && len(args) > 1 {
			n = 2
		}
		globals = append(globals, args[:n]...)
		args = args[n:]
	}
	key := strings.Join(args, " ")

	// "env-probe" reports how the CLI was configured: the global options it
	// got and the configuration variables in its environment.
	if key == "env-probe" {
		var env []string
		for _, kv := range os.Environ() {
			if strings.HasPrefix(kv, "UNITY_") || strings.HasPrefix(kv, "NO_COLOR=") {
				env = append(env, kv)
			}
		}
		data, _ := json.Marshal(map[string][]string{"globals": globals, "env": env})
		fmt.Printf(`{"type":"result","success":true,"command":"env-probe","data":%s}`+"\n", data)
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
		w := bufio.NewWriter(os.Stdout)
		for i := 1; i < resp.Repeat; i++ {
			fmt.Fprintln(w, resp.Lines[0])
		}
		w.Flush()
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
