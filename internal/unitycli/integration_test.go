package unitycli

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestRealCLI runs read-only commands through the real Unity CLI. It is skipped
// unless LAZYUNITY_REAL_CLI=1, and never changes anything.
func TestRealCLI(t *testing.T) {
	if os.Getenv("LAZYUNITY_REAL_CLI") != "1" {
		t.Skip("set LAZYUNITY_REAL_CLI=1 to run against the real Unity CLI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	st := Probe(ctx, DefaultEnv(), "", nil)
	if st.State == Missing {
		t.Skip("no Unity CLI installed")
	}
	t.Logf("CLI %s at %s (%s)", st.Version, st.Path, st.State)
	c := New(st.Path)

	ps, err := c.Projects(ctx)
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	t.Logf("%d projects", len(ps))

	eds, err := c.Editors(ctx)
	if err != nil {
		t.Fatalf("Editors: %v", err)
	}
	t.Logf("%d editors", len(eds))
	if len(eds) > 0 {
		mods, err := c.Modules(ctx, eds[0].Version, eds[0].Architecture)
		if err != nil {
			t.Fatalf("Modules: %v", err)
		}
		t.Logf("%s has %d modules", eds[0].Version, len(mods))
	}

	if _, err := c.InstallPath(ctx); err != nil {
		t.Errorf("InstallPath: %v", err)
	}
	if info, err := c.AuthStatus(ctx); err != nil {
		t.Errorf("AuthStatus: %v", err)
	} else {
		t.Logf("signed in: %v", info.LoggedIn)
	}

	page, err := c.Releases(ctx, "lts", 0, 5)
	if err != nil || len(page.Items) == 0 || page.NextOffset != 5 {
		t.Errorf("Releases: %+v %v", page, err)
	}

	// a failure must come back typed
	_, err = c.ProjectInfo(ctx, "/definitely/not/a/project")
	if e, ok := err.(*Error); !ok || !e.NotFound() {
		t.Errorf("ProjectInfo on a missing path: %v", err)
	}
}
