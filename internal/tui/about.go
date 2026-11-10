package tui

import (
	tea "charm.land/bubbletea/v2"
)

const (
	authorName = "Ilia Petrov-Komotskii (Mane Function)"
	repoURL    = "https://github.com/ManeFunction/lazyunity"
	sponsorURL = "https://github.com/sponsors/ManeFunction"
	// donateLabel is the link at the right end of the bottom line, as in lazygit.
	donateLabel = "Donate"
)

// openURL opens a web page in the browser.
func (m *Model) openURL(url string) tea.Cmd {
	if err := m.host.OpenURL(url); err != nil {
		m.toastf(toastError, "Could not open the browser: %v", err)
		return nil
	}
	m.toastf(toastInfo, "Opened %s", url)
	return nil
}

// onDonate reports whether a click at x, y is on the Donate link of the
// bottom line, which ends the line.
func (m *Model) onDonate(x, y int) bool {
	b := m.geo.bar
	if b.empty() || y != b.y || m.filter.active {
		return false
	}
	return x >= b.x+b.w-width(donateLabel) && x < b.x+b.w
}
