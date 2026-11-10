package unitycli

import "strings"

// ReleaseStream is a stream of Editor releases.
type ReleaseStream struct {
	// ID is the value `unity releases --stream` takes; "" lists every stream.
	ID    string
	Label string // its name on screen
	Desc  string
}

// StreamSupported is the stream of the current Unity versions. Unity once
// called it Tech, and --stream still does.
const StreamSupported = "tech"

// ReleaseStreams are the streams that can be listed, in the order a menu
// offers them.
var ReleaseStreams = []ReleaseStream{
	{ID: StreamSupported, Label: "Supported", Desc: "Supported releases: the current Unity versions (the default)"},
	{ID: "lts", Label: "LTS", Desc: "Long-term support releases"},
	{ID: "beta", Label: "Beta", Desc: "Pre-releases of the next version"},
	{ID: "alpha", Label: "Alpha", Desc: "Early previews"},
	{ID: "", Label: "All", Desc: "Every release stream"},
}

// StreamLabel names a stream given as a --stream value or as a release's
// stream field (LTS, TECH, SUPPORTED, BETA ...). known is false for a stream
// not in ReleaseStreams, whose name is then only capitalised.
func StreamLabel(stream string) (label string, known bool) {
	s := strings.ToLower(stream)
	if s == "supported" {
		s = StreamSupported
	}
	for _, x := range ReleaseStreams {
		if x.ID == s {
			return x.Label, true
		}
	}
	return strings.ToUpper(s[:1]) + s[1:], false
}
