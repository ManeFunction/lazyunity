package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

func TestInventoryRoot(t *testing.T) {
	inv := newInventory("linux", "/home/u", "")
	want := filepath.Clean("/home/u/Unity/Hub/Editor")
	if got := inv.installRoot(); got != want {
		t.Errorf("the default install folder: %q", got)
	}
	inv.setInstallRoot("/srv/editors")
	if got := inv.installRoot(); got != "/srv/editors" {
		t.Errorf("the CLI's install folder takes over: %q", got)
	}
}

func TestInventoryScanAndMerge(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "6000.6.8f1", unity.ExecutableRel("linux"))
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, nil, 0o755)
	inv := newInventory("linux", "", root)

	inv.setFound(scanEditors("linux", inv.installRoot()))
	if !inv.has("6000.6.8f1") || inv.find("6000.6.8f1").Location != exe {
		t.Errorf("the scan finds the Editor: %+v", inv.editors())
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

func TestInventoryMergeDarwinAppLocations(t *testing.T) {
	// The macOS CLI reports the app bundle as the Editor's location, while a
	// scan finds the executable inside it: the two are one Editor.
	root := t.TempDir()
	exe := filepath.Join(root, "6000.6.8f1", unity.ExecutableRel("darwin"))
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, nil, 0o755)
	inv := newInventory("darwin", "", root)

	inv.setFound(scanEditors("darwin", inv.installRoot()))
	inv.setListed(listEditors("darwin", []unitycli.Editor{
		{Version: "6000.6.8f1", Architecture: "x86_64", Location: filepath.Join(root, "6000.6.8f1", "Unity.app"), Default: true},
	}))
	eds := inv.editors()
	if len(eds) != 1 || eds[0].Location != exe || !eds[0].Default || eds[0].Arch != "x86_64" {
		t.Errorf("the bundle and the executable merge into one Editor: %+v", eds)
	}
	if got := inv.strays(); got != nil {
		t.Errorf("an Editor the CLI lists is not a stray: %q", got)
	}
}

func TestInventoryStraysAreToldOnce(t *testing.T) {
	inv := newInventory("linux", "", "/r")
	inv.setFound([]foundEditor{
		{Installed: unity.Installed{Version: "a", Location: "/r/a/Editor/Unity"}},
		{Installed: unity.Installed{Version: "b", Location: "/r/b/Editor/Unity"}},
	})
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
