package tui

import (
	tea "charm.land/bubbletea/v2"
)

const (
	authorName = "Ilia Petrov-Komotskii (Mane Function)"
	repoURL    = "https://github.com/ManeFunction/lazyunity"
	sponsorURL = "https://github.com/sponsors/ManeFunction"
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
