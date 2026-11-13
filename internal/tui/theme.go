// Package tui is lazyunity's terminal interface.
package tui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
)

// borderChars are the characters a frame is drawn with.
type borderChars struct{ tl, tr, bl, br, h, v string }

func borderFor(name string) borderChars {
	switch name {
	case "single":
		return borderChars{"┌", "┐", "└", "┘", "─", "│"}
	case "double":
		return borderChars{"╔", "╗", "╚", "╝", "═", "║"}
	case "bold":
		return borderChars{"┏", "┓", "┗", "┛", "━", "┃"}
	case "hidden":
		return borderChars{" ", " ", " ", " ", " ", " "}
	}
	return borderChars{"╭", "╮", "╰", "╯", "─", "│"}
}

// Theme is every style the interface uses, built from the configuration.
type Theme struct {
	Border borderChars

	ActiveBorder    lipgloss.Style
	InactiveBorder  lipgloss.Style
	SearchingBorder lipgloss.Style
	OptionsText     lipgloss.Style // the hint line

	// Selected and SelectedInactive are base styles for a cursor row. Segments
	// of the row inherit them, so the background reaches the whole line.
	Selected         lipgloss.Style
	SelectedInactive lipgloss.Style

	Text    lipgloss.Style
	Dim     lipgloss.Style
	Bold    lipgloss.Style
	Pending lipgloss.Style
	Missing lipgloss.Style
	Pinned  lipgloss.Style

	// fixed accents, independent of the configuration
	Key     lipgloss.Style // keys in menus
	Success lipgloss.Style
	Warning lipgloss.Style
	BarEdge lipgloss.Style // the brackets of a progress bar

	// palette is the terminal's own RGB for the 16 ANSI colours, as far as it
	// answered the query at start; nil entries were not reported.
	palette [16]color.Color
}

var colorNames = map[string]int{
	"black": 0, "red": 1, "green": 2, "yellow": 3, "blue": 4, "magenta": 5, "cyan": 6, "white": 7,
	"brightblack": 8, "brightred": 9, "brightgreen": 10, "brightyellow": 11,
	"brightblue": 12, "brightmagenta": 13, "brightcyan": 14, "brightwhite": 15,
}

// parseColor reads a colour name or '#rrggbb'. nil means the terminal default.
func parseColor(s string) (color.Color, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "default" || s == "" {
		return nil, nil
	}
	if n, ok := colorNames[s]; ok {
		return lipgloss.Color(strconv.Itoa(n)), nil
	}
	if strings.HasPrefix(s, "#") && len(s) == 7 {
		if _, err := strconv.ParseUint(s[1:], 16, 32); err == nil {
			return lipgloss.Color(s), nil
		}
	}
	return nil, fmt.Errorf("unknown colour %q", s)
}

// styleFrom builds a style from a lazygit-style list such as [blue, bold]. When
// background is set the colour fills the background instead of colouring text.
func styleFrom(spec []string, background bool) (lipgloss.Style, error) {
	st := lipgloss.NewStyle()
	for _, item := range spec {
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "bold":
			st = st.Bold(true)
		case "dim", "faint":
			st = st.Faint(true)
		case "underline":
			st = st.Underline(true)
		case "reverse":
			st = st.Reverse(true)
		case "strikethrough":
			st = st.Strikethrough(true)
		default:
			c, err := parseColor(item)
			if err != nil {
				return st, err
			}
			if c == nil {
				continue
			}
			if background {
				st = st.Background(c)
			} else {
				st = st.Foreground(c)
			}
		}
	}
	return st, nil
}

// NewTheme builds the theme from the GUI configuration.
func NewTheme(g config.GUI) (*Theme, error) {
	t := &Theme{Border: borderFor(g.Border)}
	var firstErr error
	get := func(name string, spec []string, background bool) lipgloss.Style {
		st, err := styleFrom(spec, background)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("gui.theme.%s: %w", name, err)
		}
		return st
	}
	th := g.Theme
	t.ActiveBorder = get("activeBorderColor", th.ActiveBorderColor, false)
	t.InactiveBorder = get("inactiveBorderColor", th.InactiveBorderColor, false)
	t.SearchingBorder = get("searchingActiveBorderColor", th.SearchingActiveBorderColor, false)
	t.OptionsText = get("optionsTextColor", th.OptionsTextColor, false)
	t.Selected = get("selectedLineBgColor", th.SelectedLineBgColor, true).
		Inherit(get("selectedLineFgColor", th.SelectedLineFgColor, false))
	t.SelectedInactive = get("inactiveViewSelectedLineBgColor", th.InactiveViewSelectedLineBgColor, true)
	t.Pending = get("pendingColor", th.PendingColor, false)
	t.Missing = get("missingColor", th.MissingColor, false)
	t.Pinned = get("pinnedColor", th.PinnedColor, false)
	t.Text = get("defaultFgColor", th.DefaultFgColor, false)
	t.Dim = lipgloss.NewStyle().Faint(true)
	t.Bold = lipgloss.NewStyle().Bold(true)
	t.Key = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	t.Success = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	t.Warning = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	t.BarEdge = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	return t, firstErr
}
