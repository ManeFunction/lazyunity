// Package unity reads Unity's files on disk: Editor versions, installed
// Editors and their platform modules. Nothing here starts the Unity CLI, so it
// is fast enough to run before the first screen is drawn.
package unity

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Version is a Unity Editor version such as 6000.6.8f1 or 2022.3.1b2.
type Version struct {
	Major, Minor, Patch int
	Type                byte // 'a' alpha, 'b' beta, 'f' final, 'p' patch
	Build               int
	Custom              int // the trailing cN some releases carry, e.g. 2019.4.1f1c1
}

var versionRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)([abfp])(\d+)(?:c(\d+))?$`)

// ParseVersion parses a version string. It expects the full version: the CLI's
// short aliases (6.6.8f1 for 6000.6.8f1) look like Unity 5-era versions and must
// not be passed in; lazyunity only reads full versions, from folder names and
// from the CLI's "version" field.
func ParseVersion(s string) (Version, bool) {
	m := versionRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Version{}, false
	}
	v := Version{Type: m[4][0]}
	v.Major, _ = strconv.Atoi(m[1])
	v.Minor, _ = strconv.Atoi(m[2])
	v.Patch, _ = strconv.Atoi(m[3])
	v.Build, _ = strconv.Atoi(m[5])
	v.Custom, _ = strconv.Atoi(m[6])
	return v, true
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d%c%d", v.Major, v.Minor, v.Patch, v.Type, v.Build)
	if v.Custom > 0 {
		s += fmt.Sprintf("c%d", v.Custom)
	}
	return s
}

// typeRank orders release types: alpha < beta < final < patch.
func typeRank(t byte) int {
	switch t {
	case 'a':
		return 0
	case 'b':
		return 1
	case 'f':
		return 2
	case 'p':
		return 3
	}
	return -1
}

// Compare returns -1, 0 or 1.
func (v Version) Compare(o Version) int {
	for _, p := range [][2]int{
		{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch},
		{typeRank(v.Type), typeRank(o.Type)}, {v.Build, o.Build}, {v.Custom, o.Custom},
	} {
		switch {
		case p[0] < p[1]:
			return -1
		case p[0] > p[1]:
			return 1
		}
	}
	return 0
}

// Less is Compare(o) < 0.
func (v Version) Less(o Version) bool { return v.Compare(o) < 0 }

// CompareStrings orders two version strings. A string that is not a version
// sorts before every version, and such strings compare as text.
func CompareStrings(a, b string) int {
	va, okA := ParseVersion(a)
	vb, okB := ParseVersion(b)
	switch {
	case okA && okB:
		return va.Compare(vb)
	case okA:
		return 1
	case okB:
		return -1
	}
	return strings.Compare(a, b)
}

// SortNewestFirst sorts version strings, newest first.
func SortNewestFirst(versions []string) {
	sort.SliceStable(versions, func(i, j int) bool { return CompareStrings(versions[i], versions[j]) > 0 })
}

// AtLeast reports whether version s is the same as or newer than major.minor
// (for example AtLeast("6000.7.0f1", 6000, 7)). Unparseable versions are not.
func AtLeast(s string, major, minor int) bool {
	v, ok := ParseVersion(s)
	if !ok {
		return false
	}
	return v.Major > major || (v.Major == major && v.Minor >= minor)
}

// Line is the release line, such as "2022.3" or "6000.6".
func (v Version) Line() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

// IsPrerelease is true for alphas and betas.
func (v Version) IsPrerelease() bool { return v.Type == 'a' || v.Type == 'b' }

// ProjectVersionFile is where a project records its Editor version.
var ProjectVersionFile = filepath.Join("ProjectSettings", "ProjectVersion.txt")

// ReadProjectVersion returns the Editor version recorded by the project at
// path, from ProjectSettings/ProjectVersion.txt.
func ReadProjectVersion(path string) (string, error) {
	f, err := os.Open(filepath.Join(path, ProjectVersionFile))
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text()) // also drops the \r of CRLF files
		if rest, ok := strings.CutPrefix(line, "m_EditorVersion:"); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no m_EditorVersion in %s", filepath.Join(path, ProjectVersionFile))
}
