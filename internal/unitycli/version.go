package unitycli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a Unity CLI version such as 1.0.0 or 1.0.0-beta.16.
type Version struct {
	Major, Minor, Patch int
	Pre                 string // "beta", "rc" ... empty for a release
	PreNum              int
}

// The verified range: the Unity CLI releases this lazyunity was checked
// against, from MinVersion up to the last patch of MaxLine. A patch release is
// trusted not to change the commands lazyunity uses, so every patch of a
// verified line is verified. A newer minor or major release may change them: it
// still runs, but is shown as not verified, and only offered on request.
//
// docs/UnityCLI.md lists what was checked. Move MaxLine up only once a new
// line has been checked against that list.
var (
	// MinVersion is the oldest CLI lazyunity works with; older ones must be
	// updated.
	MinVersion = Version{Major: 1}
	// MaxLine is the newest verified release line.
	MaxLine = Line{Major: 1, Minor: 0}
)

// Line is a release line: every patch of Major.Minor.
type Line struct{ Major, Minor int }

// Line is the release line v belongs to.
func (v Version) Line() Line { return Line{v.Major, v.Minor} }

// Compare returns -1, 0 or 1.
func (l Line) Compare(o Line) int {
	switch {
	case l.Major != o.Major:
		return cmpInt(l.Major, o.Major)
	default:
		return cmpInt(l.Minor, o.Minor)
	}
}

func (l Line) String() string { return fmt.Sprintf("%d.%d.*", l.Major, l.Minor) }

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Verified reports whether v is in the verified range.
func (v Version) Verified() bool {
	return v.Compare(MinVersion) >= 0 && v.Line().Compare(MaxLine) <= 0
}

// BeyondVerified reports whether v is newer than the verified range.
func (v Version) BeyondVerified() bool { return v.Line().Compare(MaxLine) > 0 }

// VerifiedRange names the verified range, such as "1.0.*" or "1.0.2 – 1.1.*".
func VerifiedRange() string {
	from := MinVersion.String()
	if MinVersion.Patch == 0 && MinVersion.Pre == "" {
		if MinVersion.Line() == MaxLine {
			return MaxLine.String()
		}
		from = MinVersion.Line().String()
	}
	return from + " – " + MaxLine.String()
}

var versionRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([A-Za-z]+)\.(\d+))?$`)

// ParseVersion parses "1.0.0" or "1.0.0-beta.16".
func ParseVersion(s string) (Version, bool) {
	m := versionRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Version{}, false
	}
	v := Version{Pre: strings.ToLower(m[4])}
	v.Major, _ = strconv.Atoi(m[1])
	v.Minor, _ = strconv.Atoi(m[2])
	v.Patch, _ = strconv.Atoi(m[3])
	v.PreNum, _ = strconv.Atoi(m[5])
	return v, true
}

// parseVersionOutput finds the version in the output of `unity --version`.
func parseVersionOutput(out string) (Version, bool) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if v, ok := ParseVersion(fields[len(fields)-1]); ok {
			return v, true
		}
	}
	return Version{}, false
}

// Compare returns -1, 0 or 1. A release is newer than any pre-release of the
// same number; pre-releases compare by tag (alpha < beta < rc) then number.
func (v Version) Compare(o Version) int {
	for _, p := range [][2]int{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if p[0] != p[1] {
			if p[0] < p[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case v.Pre == o.Pre:
		// fall through to the number
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	case v.Pre < o.Pre:
		return -1
	default:
		return 1
	}
	switch {
	case v.PreNum < o.PreNum:
		return -1
	case v.PreNum > o.PreNum:
		return 1
	}
	return 0
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += fmt.Sprintf("-%s.%d", v.Pre, v.PreNum)
	}
	return s
}
