package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// fakeResp is what the fake CLI answers to one argument list.
type fakeResp struct {
	File   string   `json:"file,omitempty"` // a fixture from testdata/cli
	Stdout string   `json:"stdout,omitempty"`
	Lines  []string `json:"lines,omitempty"`
	Stderr string   `json:"stderr,omitempty"`
	Exit   int      `json:"exit,omitempty"`
}

const okJSON = `{"success":true,"command":"x","data":{}}`

// useFakeCLI gives the model a CLI that is this test binary playing `unity`. Any
// argument list not in the table makes it exit 99, which the model shows as an
// error, so a test notices a command it did not expect.
func useFakeCLI(t *testing.T, m *Model, table map[string]fakeResp) *unitycli.Client {
	t.Helper()
	data, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}
	c := unitycli.NewWithCommand("unity", func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := append([]string{"-test.run=^TestFakeUnity$", "--"}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "FAKE_UNITY=1", "FAKE_UNITY_TABLE="+string(data), "GORACE=atexit_sleep_ms=0")
		return cmd
	})
	m.cli = c
	return c
}

// TestFakeUnity is the fake `unity`. It is not a test.
func TestFakeUnity(t *testing.T) {
	if os.Getenv("FAKE_UNITY") != "1" {
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
	fmt.Fprint(os.Stderr, resp.Stderr)
	os.Exit(resp.Exit)
}

// calls lists the commands the model has run: their arguments, unquoted.
func calls(c *unitycli.Client) []string {
	var out []string
	for _, e := range c.Log() {
		out = append(out, strings.Join(e.Args, " "))
	}
	return out
}

func called(c *unitycli.Client, command string) bool {
	for _, got := range calls(c) {
		if got == command {
			return true
		}
	}
	return false
}

// editorsResponse is what `unity editors --installed` says when it knows exactly
// the Editors the model has found on disk.
func editorsResponse(m *Model) fakeResp {
	var items []string
	for _, e := range m.editors {
		items = append(items, fmt.Sprintf(`{"version":%q,"architecture":%q,"location":%q,"moduleNames":[],"default":%v}`,
			e.Version, e.Arch, e.Location, e.Default))
	}
	return fakeResp{Stdout: `{"success":true,"command":"editors","data":{"items":[` + strings.Join(items, ",") + `],"total":` +
		fmt.Sprint(len(items)) + `}}`}
}
