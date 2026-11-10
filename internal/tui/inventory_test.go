package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

func TestInventoryRoots(t *testing.T) {
	extra := []string{"/opt/unity", "/opt/unity/", "", "/srv/editors"}
	inv := newInventory("linux", "/home/u", "", func() []string { return extra })
	want := []string{filepath.Clean("/home/u/Unity/Hub/Editor"), "/opt/unity", "/srv/editors"}
	if got := inv.roots(); !reflect.DeepEqual(got, want) {
		t.Errorf("the default install folder, then each extra folder once: %q", got)
	}
	inv.setInstallRoot("/srv/editors")
	if got := inv.roots(); !reflect.DeepEqual(got, []string{"/srv/editors", "/opt/unity"}) {
		t.Errorf("the CLI's install folder comes first, and only once: %q", got)
	}
}

func TestInventoryScanAndMerge(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "6000.6.8f1", unity.ExecutableRel("linux"))
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, nil, 0o755)
	gone := filepath.Join(root, "gone")
	inv := newInventory("linux", "", root, func() []string { return []string{gone} })

	inv.setFound(scanEditors("linux", inv.roots()))
	if !inv.has("6000.6.8f1") || inv.find("6000.6.8f1").Location != exe || inv.foundIn(root) != 1 {
		t.Errorf("the scan finds the Editor: %+v", inv.editors())
	}
	if !inv.exists(root) || inv.exists(gone) {
		t.Error("the scan knows which folders are there")
	}
	inv.setListed(listEditors("linux", []unitycli.Editor{
		{Version: "6000.6.8f1", Location: exe, Default: true},
		{Version: "2022.3.62f1", Location: "/elsewhere/2022.3.62f1/Editor/Unity"},
	}))
	eds := inv.editors()
	if len(eds) != 2 || eds[0].Version != "6000.6.8f1" || !eds[0].Default || eds[1].Dir != "/elsewhere/2022.3.62f1" {
		t.Errorf("the CLI marks the default and adds the Editors it knows elsewhere: %+v", eds)
	}
}

func TestInventoryStraysAreToldOnce(t *testing.T) {
	inv := newInventory("linux", "", "/r", nil)
	inv.setFound([]foundEditor{
		{Installed: unity.Installed{Version: "a", Location: "/r/a/Editor/Unity"}},
		{Installed: unity.Installed{Version: "b", Location: "/r/b/Editor/Unity"}},
	}, nil)
	if got := inv.strays(); got != nil {
		t.Errorf("before the CLI listed its Editors there is nothing to tell it: %q", got)
	}
	inv.setListed([]listedEditor{{Editor: unitycli.Editor{Version: "a", Location: "/r/a/Editor/Unity"}}})
	got := inv.strays()
	if !reflect.DeepEqual(got, []string{"/r/b/Editor/Unity"}) {
		t.Fatalf("only what the CLI does not list: %q", got)
	}
	inv.told(got)
	if again := inv.strays(); again != nil {
		t.Errorf("each Editor is told once: %q", again)
	}
}

func TestMergeEditorsTakesTheVersionFromTheCLI(t *testing.T) {
	found := []foundEditor{{Installed: unity.Installed{Version: "unity-2022.3", Location: "/opt/unity-2022.3/Editor/Unity", Dir: "/opt/unity-2022.3"}}}
	listed := []listedEditor{{Editor: unitycli.Editor{Version: "2022.3.62f1", Location: "/opt/unity-2022.3/Editor/Unity", Architecture: "x86_64", Default: true}}}
	got := mergeEditors(found, listed)
	if len(got) != 1 {
		t.Fatalf("one Editor, listed once: %+v", got)
	}
	if e := got[0]; e.Version != "2022.3.62f1" || !e.Default || e.Arch != "x86_64" {
		t.Errorf("the CLI knows the real version of an Editor in a folder not named after it: %+v", e)
	}
}
