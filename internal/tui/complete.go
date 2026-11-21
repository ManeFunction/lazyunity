package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

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
