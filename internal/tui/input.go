package tui

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// input is a single-line text field with a cursor.
type input struct {
	value []rune
	cur   int
}

func newInput(s string) *input {
	in := &input{}
	in.Set(s)
	return in
}

// String is the text.
func (in *input) String() string { return string(in.value) }

// Set replaces the text and puts the cursor at the end.
func (in *input) Set(s string) {
	in.value = []rune(s)
	in.cur = len(in.value)
}

// Paste inserts text at the cursor; line breaks become spaces.
func (in *input) Paste(s string) {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s)
	in.insert([]rune(s))
}

func (in *input) insert(r []rune) {
	out := make([]rune, 0, len(in.value)+len(r))
	out = append(out, in.value[:in.cur]...)
	out = append(out, r...)
	out = append(out, in.value[in.cur:]...)
	in.value = out
	in.cur += len(r)
}

func (in *input) deleteRange(from, to int) {
	if from < 0 || to > len(in.value) || from >= to {
		return
	}
	in.value = append(in.value[:from:from], in.value[to:]...)
	in.cur = from
}

// wordStart is where the word before the cursor begins.
func (in *input) wordStart() int {
	i := in.cur
	for i > 0 && unicode.IsSpace(in.value[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(in.value[i-1]) {
		i--
	}
	return i
}

// Key handles a key press and reports whether the field used it.
func (in *input) Key(msg tea.KeyPressMsg) bool {
	k := msg.Key()
	if k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		in.insert([]rune(strings.NewReplacer("\n", " ", "\r", " ").Replace(k.Text)))
		return true
	}
	switch msg.String() {
	case "backspace":
		if in.cur > 0 {
			in.deleteRange(in.cur-1, in.cur)
		}
	case "delete", "ctrl+d":
		in.deleteRange(in.cur, in.cur+1)
	case "left", "ctrl+b":
		in.cur = max(in.cur-1, 0)
	case "right", "ctrl+f":
		in.cur = min(in.cur+1, len(in.value))
	case "home", "ctrl+a":
		in.cur = 0
	case "end", "ctrl+e":
		in.cur = len(in.value)
	case "ctrl+u":
		in.deleteRange(0, in.cur)
	case "ctrl+k":
		in.deleteRange(in.cur, len(in.value))
	case "ctrl+w", "alt+backspace":
		in.deleteRange(in.wordStart(), in.cur)
	default:
		return false
	}
	return true
}

// View draws the field in w cells, with the cursor as a reversed cell.
func (in *input) View(w int) string { return in.ViewGhost(w, "", lipgloss.Style{}) }

// ViewGhost is View with a suggestion drawn in ghost style after the text, as
// editors show a completion. The cursor sits on its first character. It is
// shown only while the cursor is at the end of the text.
func (in *input) ViewGhost(w int, ghost string, ghostStyle lipgloss.Style) string {
	if w < 2 {
		return ""
	}
	start := 0
	if in.cur > w-1 {
		start = in.cur - (w - 1)
	}
	end := min(start+w-1, len(in.value))
	before := string(in.value[start:min(in.cur, end)])
	after := ""
	cursor := " "
	if in.cur < len(in.value) {
		cursor = string(in.value[in.cur])
		after = string(in.value[in.cur+1 : end])
	}
	if g := []rune(ghost); len(g) > 0 && in.cur == len(in.value) {
		g = g[:min(len(g), w-1-len([]rune(before)))]
		if len(g) > 0 {
			rest := ""
			if len(g) > 1 {
				rest = ghostStyle.Render(string(g[1:]))
			}
			return padRight(before+ghostStyle.Reverse(true).Render(string(g[0]))+rest, w)
		}
	}
	out := before + lipgloss.NewStyle().Reverse(true).Render(cursor) + after
	return padRight(out, w)
}
