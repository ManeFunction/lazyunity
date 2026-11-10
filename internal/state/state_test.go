package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Load(dir)
	s.AddRoot("/opt/unity")
	s.SetPending("/p/game", Pending{Version: "6000.6.8f1", BuildTarget: "Android"})
	s.Sort = SortName
	s.DefaultProjectDir = "/games"
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	got := Load(dir)
	if !reflect.DeepEqual(got.ExtraRoots, []string{"/opt/unity"}) ||
		got.PendingFor("/p/game") != (Pending{Version: "6000.6.8f1", BuildTarget: "Android"}) ||
		got.Sort != SortName || got.DefaultProjectDir != "/games" {
		t.Errorf("state did not survive a round trip: %+v", got)
	}
	if got.Path() != filepath.Join(dir, "state.yml") {
		t.Errorf("Path = %q", got.Path())
	}
}

func TestSaveCreatesDirAndLeavesNoTempFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "lazyunity")
	s := Load(dir)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.yml" {
		t.Errorf("unexpected files: %v", entries)
	}
}

func TestLoadNeverFails(t *testing.T) {
	// missing
	if s := Load(t.TempDir()); s == nil || len(s.ExtraRoots) != 0 {
		t.Error("a missing file must give an empty state")
	}
	// corrupt: starts fresh, but still knows where to save
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "state.yml"), []byte("extraRoots: [unclosed"), 0o644)
	s := Load(dir)
	if len(s.ExtraRoots) != 0 || s.Path() != filepath.Join(dir, "state.yml") {
		t.Errorf("corrupt file: %+v", s)
	}
	if err := s.Save(); err != nil {
		t.Errorf("a state loaded from a corrupt file must be saveable: %v", err)
	}
}

func TestPending(t *testing.T) {
	s := Load(t.TempDir())
	if !s.PendingFor("/x").Empty() {
		t.Error("nothing is pending at first")
	}
	s.SetPending("/x", Pending{BuildTarget: "iOS"})
	if s.PendingFor("/x").Empty() || s.PendingFor("/x").BuildTarget != "iOS" {
		t.Error("pending not recorded")
	}
	s.SetPending("/x", Pending{}) // clearing removes the entry
	if _, ok := s.Pending["/x"]; ok {
		t.Error("an empty pending must delete the entry")
	}
	s.SetPending("/never-set", Pending{}) // clearing something unknown is fine
}

func TestRoots(t *testing.T) {
	s := Load(t.TempDir())
	if !s.AddRoot("/a") || !s.AddRoot("/b") {
		t.Fatal("adding new roots must succeed")
	}
	if s.AddRoot("/a") {
		t.Error("a duplicate root must be refused")
	}
	if !s.RemoveRoot("/a") || s.RemoveRoot("/a") {
		t.Error("RemoveRoot must report whether it removed something")
	}
	if !reflect.DeepEqual(s.ExtraRoots, []string{"/b"}) {
		t.Errorf("roots = %v", s.ExtraRoots)
	}
}

func TestSortOrder(t *testing.T) {
	s := &State{}
	if s.SortOrder() != SortLastOpened {
		t.Error("default sort is last opened")
	}
	for _, v := range []string{SortName, SortVersion, SortLastOpened} {
		s.Sort = v
		if s.SortOrder() != v {
			t.Errorf("SortOrder() = %q, want %q", s.SortOrder(), v)
		}
	}
	s.Sort = "bogus"
	if s.SortOrder() != SortLastOpened {
		t.Error("an unknown sort falls back to last opened")
	}
}

func TestSaveWithoutPath(t *testing.T) {
	if err := (&State{}).Save(); err == nil {
		t.Error("saving a state with no path must fail, not write to the working directory")
	}
}
