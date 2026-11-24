package unitycli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// A change the CLI refuses for want of administrator rights is made again as
// administrator, with the same command line.
func TestRunElevatesWhenTheCLINeedsAdminRights(t *testing.T) {
	refused := fakeResp{Exit: ExitCommandFailed, Stdout: `{"success":false,"command":"uninstall","data":null,"errors":[{"code":"UNINSTALL_ELEVATION_NEEDS_TERMINAL","message":"needs an administrator"}]}`}
	c := fakeClient(t, map[string]fakeResp{"uninstall 6000.6.9f1 --yes": refused})
	var got []string
	c.elevate = func(_ context.Context, path string, args []string) (processResult, error) {
		got = append([]string{path}, args...)
		return processResult{Stdout: `{"success":true,"command":"uninstall","data":null}`}, nil
	}
	if err := c.Uninstall(ctx5(t), "6000.6.9f1", ""); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if want := "unity --format json --no-banner --no-pager --no-color --non-interactive uninstall 6000.6.9f1 --yes"; strings.Join(got, " ") != want {
		t.Errorf("elevated run: %q, want %q", strings.Join(got, " "), want)
	}
	if n := len(c.Log()); n != 2 {
		t.Errorf("both runs are logged, got %d entries", n)
	}
}

func TestElevationDeclined(t *testing.T) {
	refused := fakeResp{Exit: ExitCommandFailed, Stdout: `{"success":false,"command":"uninstall","data":null,"errors":[{"code":"UNINSTALL_ELEVATION_NEEDS_TERMINAL","message":"needs an administrator"}]}`}
	c := fakeClient(t, map[string]fakeResp{"uninstall 6000.6.9f1 --yes": refused})
	c.elevate = func(context.Context, string, []string) (processResult, error) {
		return processResult{ExitCode: -1}, errElevationDeclined
	}
	err := c.Uninstall(ctx5(t), "6000.6.9f1", "")
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeElevationDeclined {
		t.Fatalf("got %v", err)
	}
}

// Other failures, and every failure where lazyunity cannot elevate, are
// reported as they are.
func TestNoElevationForOtherErrors(t *testing.T) {
	refused := fakeResp{Exit: ExitCommandFailed, Stdout: `{"success":false,"command":"uninstall","data":null,"errors":[{"code":"UNINSTALL_ELEVATION_NEEDS_TERMINAL","message":"needs an administrator"}]}`}
	other := fakeResp{Exit: ExitNotFound, Stdout: `{"success":false,"command":"uninstall","data":null,"errors":[{"code":"EDITOR_NOT_FOUND","message":"no such Editor"}]}`}
	c := fakeClient(t, map[string]fakeResp{"uninstall 1 --yes": other, "uninstall 2 --yes": refused})
	c.elevate = func(context.Context, string, []string) (processResult, error) {
		t.Error("elevated for an error that does not ask for it")
		return processResult{}, nil
	}
	if err := c.Uninstall(ctx5(t), "1", ""); err == nil || !strings.Contains(err.Error(), "no such Editor") {
		t.Errorf("got %v", err)
	}
	c.elevate = nil
	var e *Error
	if err := c.Uninstall(ctx5(t), "2", ""); !errors.As(err, &e) || !e.NeedsElevation() {
		t.Errorf("got %v", err)
	}
}
