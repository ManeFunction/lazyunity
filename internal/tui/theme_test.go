package tui

import (
	"strings"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/config"
)

func TestParseColor(t *testing.T) {
	for _, ok := range []string{"red", "Blue", "brightblue", "#5c9dff", "#FFFFFF", "default", " cyan "} {
		if _, err := parseColor(ok); err != nil {
			t.Errorf("parseColor(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"purple", "#12345", "#gggggg", "12"} {
		if _, err := parseColor(bad); err == nil {
			t.Errorf("parseColor(%q) should fail", bad)
		}
	}
	if c, _ := parseColor("default"); c != nil {
		t.Error("default means no colour")
	}
}

func TestStyleFrom(t *testing.T) {
	st, err := styleFrom([]string{"red", "bold", "underline"}, false)
	if err != nil {
		t.Fatal(err)
	}
	out := st.Render("x")
	if !strings.Contains(out, "\x1b[") || !strings.Contains(out, "x") {
		t.Errorf("render = %q", out)
	}
	if _, err := styleFrom([]string{"blue", "shiny"}, false); err == nil {
		t.Error("an unknown word must be an error")
	}
	// background and foreground produce different output
	fg, _ := styleFrom([]string{"blue"}, false)
	bg, _ := styleFrom([]string{"blue"}, true)
	if fg.Render("x") == bg.Render("x") {
		t.Error("a background colour must differ from a foreground colour")
	}
	if got, _ := styleFrom(nil, false); got.Render("x") != "x" {
		t.Error("an empty spec is a plain style")
	}
}

func TestNewThemeDefaults(t *testing.T) {
	th, err := NewTheme(config.Default().GUI)
	if err != nil {
		t.Fatal(err)
	}
	if th.ActiveBorder.Render("x") == th.InactiveBorder.Render("x") {
		t.Error("the active and inactive borders must differ")
	}
	if th.Selected.Render("x") == "x" {
		t.Error("the cursor row must be styled")
	}
	if th.Border.tl != "╭" {
		t.Errorf("default border = %+v", th.Border)
	}
}

func TestNewThemeNamesTheBadKey(t *testing.T) {
	g := config.Default().GUI
	g.Theme.PendingColor = []string{"chartreuse"}
	if _, err := NewTheme(g); err == nil || !strings.Contains(err.Error(), "gui.theme.pendingColor") {
		t.Errorf("error should name the setting, got %v", err)
	}
}
