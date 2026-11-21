package unitycli

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// cdn answers for the versions it has.
func cdn(have ...string) (ProbeFunc, *[]string) {
	var asked []string
	return func(_ context.Context, url string) (int, error) {
		v := strings.TrimSuffix(strings.TrimPrefix(url, CDNBase), "/latest.json")
		asked = append(asked, v)
		for _, h := range have {
			if h == v {
				return http.StatusOK, nil
			}
		}
		return http.StatusNotFound, nil
	}, &asked
}

func TestLatestVerified(t *testing.T) {
	pinVerified(t, Version{Major: 1}, Line{Major: 1, Minor: 1})

	probe, asked := cdn("1.0.0", "1.0.1", "1.1.0", "1.1.1", "1.1.3", "1.2.0")
	v, ok := LatestVerified(context.Background(), probe)
	if !ok || v.String() != "1.1.3" {
		t.Errorf("got %v %v, want 1.1.3: a missing patch number is skipped", v, ok)
	}
	for _, a := range *asked {
		if strings.HasPrefix(a, "1.2.") || strings.HasPrefix(a, "1.0.") {
			t.Errorf("only the newest verified line with releases is searched, asked %v", *asked)
			break
		}
	}

	// a newest line without releases yet falls back to the line before
	probe, _ = cdn("1.0.0", "1.0.1")
	if v, ok := LatestVerified(context.Background(), probe); !ok || v.String() != "1.0.1" {
		t.Errorf("got %v %v, want 1.0.1", v, ok)
	}

	// releases older than MinVersion are not verified
	MinVersion = Version{Major: 1, Minor: 1, Patch: 2}
	probe, _ = cdn("1.1.0", "1.1.1")
	if v, ok := LatestVerified(context.Background(), probe); ok {
		t.Errorf("nothing at or above MinVersion, got %v", v)
	}

	// offline
	offline := func(context.Context, string) (int, error) { return 0, errors.New("no network") }
	if _, ok := LatestVerified(context.Background(), offline); ok {
		t.Error("an unreachable CDN finds nothing")
	}
}
