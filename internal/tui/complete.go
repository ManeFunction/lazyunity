package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
)

// completeDir completes a folder path as far as it is unambiguous.
func completeDir(text string) []string {
	out := completeExpanded(config.ExpandHome(text))
	if strings.HasPrefix(text, "~") {
		for i, o := range out {
			out[i] = displayPath(o) // keep the ~ the user typed
		}
	}
	return out
}

// validFolder is the validator of a prompt that asks for an existing folder:
// empty when text, ~ expanded, is one.
func validFolder(text string) string {
	if st, err := os.Stat(config.ExpandHome(text)); err != nil || !st.IsDir() {
		return "That folder does not exist"
	}
	return ""
}

// validNewFolder is the validator of a prompt that asks for a folder lazyunity
// may create: empty when text, ~ expanded, is a folder or could be made one.
// Only a path that cannot be a folder is refused: one that names a file, or
// goes through one.
func validNewFolder(text string) string {
	if strings.TrimSpace(text) == "" {
		return "Type a folder"
	}
	p := filepath.Clean(config.ExpandHome(text))
	if st, err := os.Stat(p); err == nil {
		if !st.IsDir() {
			return "That is a file, not a folder"
		}
		return ""
	}
	for q := filepath.Dir(p); ; q = filepath.Dir(q) {
		if st, err := os.Stat(q); err == nil {
			if !st.IsDir() {
				return "A file is in the way: " + displayPath(q)
			}
			return ""
		}
		if filepath.Dir(q) == q {
			return "" // nothing of the path exists: MkdirAll tells
		}
	}
}

// withFolder runs then once the folder dir exists. A missing one is not an
// error: the user is asked whether to create it, with its parents; if they
// decline, back (the prompt the folder was typed in, if any) comes again.
func (m *Model) withFolder(dir string, back *promptPopup, then func(*Model) tea.Cmd) tea.Cmd {
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return then(m)
	}
	m.push(&confirmPopup{
		title:   "Create the folder?",
		message: "The folder " + displayPath(dir) + " does not exist. Create it?",
		yes:     "Create",
		onYes: func(m *Model) tea.Cmd {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				m.toastf(toastError, "Could not create the folder: %v", err)
				return nil
			}
			return then(m)
		},
		onNo: func(m *Model) tea.Cmd {
			if back != nil {
				back.err = ""
				m.push(back)
			}
			return nil
		},
	})
	return nil
}

// dirMatches lists the folders (full text expanded) whose names continue text,
// and the folder they are in. Hidden folders show only once a dot is typed.
func dirMatches(text string) (dir string, names []string) {
	dir, base := filepath.Split(text)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir, nil
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), base) || (base == "" && strings.HasPrefix(e.Name(), ".")) {
			continue
		}
		if st, err := os.Stat(filepath.Join(dir, e.Name())); err != nil || !st.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return dir, names
}

func completeExpanded(text string) []string {
	dir, names := dirMatches(text)
	base := filepath.Base(text)
	if strings.HasSuffix(text, string(filepath.Separator)) || text == "" {
		base = ""
	}
	switch len(names) {
	case 0:
		return nil
	case 1:
		return []string{filepath.Join(dir, names[0]) + string(filepath.Separator)}
	}
	common := commonPrefix(names)
	if len(common) > len(base) {
		return []string{filepath.Join(dir, common)}
	}
	return nil
}

// commonPrefix is the longest prefix every name starts with, cut between
// characters, never inside one.
func commonPrefix(names []string) string {
	common := []rune(names[0])
	for _, n := range names[1:] {
		r := []rune(n)
		i := 0
		for i < len(common) && i < len(r) && common[i] == r[i] {
			i++
		}
		common = common[:i]
	}
	return string(common)
}
