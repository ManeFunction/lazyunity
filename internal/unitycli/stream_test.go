package unitycli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// installKey is the argument list Install(...) produces for the options used
// below.
const installKey = "install 6000.5.1f1 --module webgl --child-modules --accept-eula --yes"

var installOpts = InstallOptions{Version: "6000.5.1f1", Modules: []string{"webgl"}}

func TestStreamFrames(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{installKey: {Lines: []string{
		`{"type":"progress","pct":10.5,"msg":"Downloading Unity 6000.5.1f1...","phase":"download"}`,
		``,
		`not json at all`,
		`{"type":"progress","pct":100,"msg":"Installing Web Build Support...","phase":"install","uid":"webgl","parentItemUid":"editor"}`,
		`{"type":"result","success":true,"command":"install","data":{"items":[{"uid":"editor","status":"installed"}]},"errors":[]}`,
	}}})

	s, err := c.Install(ctx5(t), installOpts)
	if err != nil {
		t.Fatal(err)
	}
	var frames []Frame
	for f := range s.Frames {
		frames = append(frames, f)
	}
	if err := s.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("got %d frames, want 3 (blank and non-JSON lines are skipped): %+v", len(frames), frames)
	}
	if f := frames[0]; f.Type != "progress" || f.Phase != "download" || f.Pct == nil || *f.Pct != 10.5 ||
		f.Message != "Downloading Unity 6000.5.1f1..." {
		t.Errorf("frame 0: %+v", f)
	}
	if f := frames[1]; f.ParentItemUID != "editor" || f.UID != "webgl" || f.Phase != "install" {
		t.Errorf("frame 1: %+v", f)
	}
	if f := frames[2]; !f.IsResult() || f.Success == nil || !*f.Success || len(f.Data) == 0 {
		t.Errorf("result frame: %+v", f)
	}
	if frames[0].Raw == "" {
		t.Error("Raw should keep the printed line")
	}

	log := c.Log()
	if len(log) != 1 || log[0].ExitCode != 0 || log[0].Stdout == "" {
		t.Errorf("stream should be logged once when it ends: %+v", log)
	}
}

// TestStreamRealOutput replays what `unity install-modules` really printed.
func TestStreamRealOutput(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{
		"install-modules --editor-version 6000.6.8f1 --module language-ja --child-modules --accept-eula --yes": {File: "install-modules.ndjson"},
	})
	s, err := c.InstallModules(ctx5(t), "6000.6.8f1", "", []string{"language-ja"})
	if err != nil {
		t.Fatal(err)
	}
	var progress []Frame
	var result Frame
	for f := range s.Frames {
		if f.IsResult() {
			result = f
		} else {
			progress = append(progress, f)
		}
	}
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	if len(progress) != 6 {
		t.Fatalf("got %d progress frames, want 6", len(progress))
	}
	// the phases and percentages the interface will show
	wantPhase := []string{"download", "download", "download", "download", "install", ""}
	wantPct := []float64{0, 11, 100, 100, 50, 100}
	for i, f := range progress {
		if f.Phase != wantPhase[i] || f.Pct == nil || *f.Pct != wantPct[i] || f.Message == "" {
			t.Errorf("frame %d: %+v", i, f)
		}
	}
	if progress[1].Message != "Downloading 日本語..." {
		t.Errorf("message = %q", progress[1].Message)
	}
	if result.Success == nil || !*result.Success || result.Command != "install-modules" {
		t.Errorf("result: %+v", result)
	}
}

func TestStreamFailureResult(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{installKey: {
		Lines:  []string{`{"type":"result","success":false,"errors":[{"code":"INSTALL_LOCKED","message":"Another install is running."}]}`},
		Stderr: "details on stderr",
		Exit:   ExitInstallLocked,
	}})
	s, err := c.Install(ctx5(t), installOpts)
	if err != nil {
		t.Fatal(err)
	}
	werr := s.Wait() // Wait on its own must not deadlock, even with frames unread
	var e *Error
	if !errors.As(werr, &e) {
		t.Fatalf("err = %v", werr)
	}
	if !e.InstallLocked() || e.Code != "INSTALL_LOCKED" || e.Message != "Another install is running." || e.Output != "details on stderr" {
		t.Errorf("error: %+v", e)
	}
}

func TestStreamNoResult(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{installKey: {Lines: []string{`{"type":"progress","pct":1}`}}})
	s, err := c.Install(ctx5(t), installOpts)
	if err != nil {
		t.Fatal(err)
	}
	var e *Error
	if err := s.Wait(); !errors.As(err, &e) || e.Message == "" {
		t.Errorf("exit 0 without a result frame must be an error, got %v", err)
	}
}

func TestStreamManyFramesDoNotBlock(t *testing.T) {
	var lines []string
	for i := 0; i < 500; i++ { // more than the channel buffer
		lines = append(lines, `{"type":"progress","pct":1}`)
	}
	lines = append(lines, `{"type":"result","success":true}`)
	c := fakeClient(t, map[string]fakeResp{installKey: {Lines: lines}})

	s, err := c.Install(ctx5(t), installOpts)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Wait() }() // nobody reads Frames
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Wait deadlocked with unread frames")
	}
}

// TestStreamUnreadFramesDoNotStallTheCLI stands for lazyunity handing the
// terminal over: nobody reads Frames, yet the CLI prints far more than the pipe
// and the queue hold. It must still finish, and the frames left still end where
// the CLI did, with the result last.
func TestStreamUnreadFramesDoNotStallTheCLI(t *testing.T) {
	pad := strings.Repeat("x", 200)
	c := fakeClient(t, map[string]fakeResp{installKey: {Repeat: 2000, Lines: []string{
		`{"type":"progress","pct":1,"msg":"` + pad + `"}`,
		`{"type":"progress","phase":"install","msg":"Installing..."}`,
		`{"type":"progress","pct":99}`,
		`{"type":"result","success":true}`,
	}}})

	s, err := c.Install(ctx5(t), installOpts)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.done: // the command ended with every frame unread
	case <-time.After(15 * time.Second):
		t.Fatal("the CLI stalled while nobody read its frames")
	}
	var frames []Frame
	for f := range s.Frames {
		frames = append(frames, f)
	}
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	if n := len(frames); n > 64 || n < 2 {
		t.Fatalf("got %d frames, want the queue's worth at most", n)
	}
	// Replay the frames the way the task line does.
	var pct float64
	var phase, msg string
	for _, f := range frames[:len(frames)-1] {
		if f.IsResult() {
			t.Fatalf("the result must be the last frame: %+v", frames)
		}
		if f.Pct != nil {
			pct = *f.Pct
		}
		if f.Phase != "" {
			phase = f.Phase
		}
		if f.Message != "" {
			msg = f.Message
		}
	}
	if !frames[len(frames)-1].IsResult() {
		t.Errorf("the result frame must not be dropped, last frame: %+v", frames[len(frames)-1])
	}
	if pct != 99 || phase != "install" || msg != "Installing..." {
		t.Errorf("the frames left should end where the CLI did: pct %v, phase %q, msg %q", pct, phase, msg)
	}
}

func TestStreamCancel(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{installKey: {Sleep: "60s"}})
	ctx, cancel := context.WithCancel(context.Background())
	s, err := c.Install(ctx, installOpts)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(300*time.Millisecond, cancel)

	done := make(chan error, 1)
	go func() { done <- s.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a cancelled stream must report an error")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("cancelling did not stop the command")
	}
}

func TestStreamCannotStart(t *testing.T) {
	c := New("/definitely/not/here/unity")
	_, err := c.Install(ctx5(t), installOpts)
	var e *Error
	if !errors.As(err, &e) || e.ExitCode != -1 {
		t.Errorf("err = %v", err)
	}
}

func TestInstallModulesAndUpgradeArguments(t *testing.T) {
	ok := []string{`{"type":"result","success":true}`}
	c := fakeClient(t, map[string]fakeResp{
		"install-modules --editor-version 6000.6.8f1 --module webgl --module ios --architecture arm64 --child-modules --accept-eula --yes": {Lines: ok},
		"editors upgrade 2022.3.10f1 --yes --accept-eula":                                                           {Lines: ok},
		"install 6000.5.1f1 --changeset abc --architecture arm64 --child-modules --accept-eula --yes --set-default": {Lines: ok},
	})
	ctx := ctx5(t)

	s, err := c.InstallModules(ctx, "6000.6.8f1", "arm64", []string{"webgl", "ios"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(); err != nil {
		t.Errorf("InstallModules: %v", err)
	}
	s, err = c.UpgradeEditor(ctx, "2022.3.10f1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(); err != nil {
		t.Errorf("UpgradeEditor: %v", err)
	}
	s, err = c.Install(ctx, InstallOptions{Version: "6000.5.1f1", Changeset: "abc", Architecture: "arm64", SetDefault: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(); err != nil {
		t.Errorf("Install with options: %v", err)
	}
}

func TestFinishedStream(t *testing.T) {
	pct := 50.0
	s := FinishedStream(nil, Frame{Type: "progress", Pct: &pct})
	f, ok := <-s.Frames
	if !ok || *f.Pct != 50 {
		t.Fatalf("the frame is there to read: %+v %v", f, ok)
	}
	if _, ok := <-s.Frames; ok {
		t.Error("and then the stream is closed")
	}
	if s.Wait() != nil {
		t.Error("a stream finished without an error waits with none")
	}
	failed := &Error{ExitCode: 1}
	if FinishedStream(failed).Wait() != failed {
		t.Error("Wait returns the stream's error")
	}
}
