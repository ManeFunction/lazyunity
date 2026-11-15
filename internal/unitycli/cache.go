package unitycli

import (
	"os"
	"strings"
)

// CachedBytes is how much of a version's install is already in the download
// cache folder dir: the sizes of the files left there by an interrupted
// install. The CLI keeps them (Unity-6000.6.9f1.tar.xz) and a later install of
// the same version carries on from where they stop; a finished install
// removes them. Only the top of the folder counts: the subfolders hold the
// downloads of --download-directory installs, which a plain install does not
// reuse. A folder that cannot be read has nothing cached.
func CachedBytes(dir, version string) int64 {
	if dir == "" || version == "" {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var n int64
	for _, e := range entries {
		if !e.Type().IsRegular() || !namesVersion(e.Name(), version) {
			continue
		}
		if info, err := e.Info(); err == nil {
			n += info.Size()
		}
	}
	return n
}

// namesVersion reports whether a file name holds the version as a whole word:
// Unity-6000.6.9f1.tar.xz names 6000.6.9f1, but not 6000.6.9f10 or 16000.6.9f1.
func namesVersion(name, version string) bool {
	for i := 0; ; {
		j := strings.Index(name[i:], version)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(version)
		before := start == 0 || !isWordByte(name[start-1]) && name[start-1] != '.'
		after := end == len(name) || !isWordByte(name[end])
		if before && after {
			return true
		}
		i = start + 1
	}
}

func isWordByte(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
