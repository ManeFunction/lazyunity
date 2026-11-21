package unitycli

import (
	"context"
	"net/http"
	"time"
)

// CDNBase is where Unity publishes the CLI. Each release has a manifest of its
// own at <CDNBase><version>/latest.json; `self-update --target` and the
// installer's UNITY_CLI_VERSION read the same file.
const CDNBase = "https://public-cdn.cloud.unity3d.com/hub/prod/cli/"

// ProbeFunc fetches url and returns the HTTP status code. An error means the
// CDN could not be reached.
type ProbeFunc func(ctx context.Context, url string) (int, error)

// maxMisses is how many missing patch numbers in a row end the search of a line.
const maxMisses = 3

// maxPatch bounds the search of a line.
const maxPatch = 200

// LatestVerified finds the newest CLI release in the verified range. The CDN
// has no list of versions, so it asks for the manifest of each patch of each
// verified line, newest line first, until a few in a row are missing. ok is
// false when no verified release was found, or the CDN could not be reached.
func LatestVerified(ctx context.Context, probe ProbeFunc) (Version, bool) {
	if probe == nil {
		probe = headStatus
	}
	for l := MaxLine; l.Compare(MinVersion.Line()) >= 0 && l.Minor >= 0; l.Minor-- {
		best, found, misses := Version{}, false, 0
		for p := 0; p <= maxPatch && misses < maxMisses; p++ {
			v := Version{Major: l.Major, Minor: l.Minor, Patch: p}
			code, err := probe(ctx, CDNBase+v.String()+"/latest.json")
			if err != nil {
				return Version{}, false
			}
			if code != http.StatusOK {
				misses++
				continue
			}
			misses = 0
			if v.Verified() {
				best, found = v, true
			}
		}
		if found {
			return best, true
		}
	}
	return Version{}, false
}

// headStatus asks for url's headers only.
func headStatus(ctx context.Context, url string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}
