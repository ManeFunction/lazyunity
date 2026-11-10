package unitycli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a Unity CLI version such as 1.0.0-beta.16.
type Version struct {
	Major, Minor, Patch int
	Pre                 string // "beta", "rc" ... empty for a release
	PreNum              int
}

// MinVersion is the oldest CLI lazyunity supports. Every command it uses was
// tested against this release.
var MinVersion = Version{Major: 1, Pre: "beta", PreNum: 16}

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
