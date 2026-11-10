package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// releasesTab is the Releases tab: the Editor versions there are to download,
// from one release stream. It loads a page at a time, from when it is first
// looked at; while a filter is typed it loads every page, so that the filter
// sees every release. The generic panel code knows none of this: it reaches
// the tab through the tab interface.
type releasesTab struct {
	*listTab[unitycli.Release]

	stream     string // "" (every stream), or a unitycli.ReleaseStreams ID
	nextOffset int    // 0 when there is no more
	loaded     bool
	busy       bool // a page is on its way
	// gen counts the times the list was started over, so that a page asked for
	// before is not added to the new list.
	gen int
}

type releasesMsg struct {
	stream string
	gen    int
	offset int
	page   unitycli.ReleasePage
	err    error
}

const (
	releasesPerPage = 25
	// releasesPerSearch is the page size while a filter is typed: every page is
	// then loaded, so that the filter sees every release, in fewer calls.
	releasesPerSearch = 200
)

func newReleasesTab(stream string) *releasesTab {
	t := &releasesTab{
		listTab: &listTab[unitycli.Release]{
			list:       newList(func(r unitycli.Release) string { return r.Version + " " + r.Stream }, func(r unitycli.Release) string { return r.Version + "/" + r.Architecture }),
			name:       "Releases",
			context:    ctxReleases,
			empty:      "Loading releases…",
			filterLine: true,
		},
		stream: stream,
	}
	t.draw = func(m *Model, r unitycli.Release, w int, base lipgloss.Style) string {
		return m.drawRelease(r, w, base)
	}
	return t
}

// ---- the tab hooks -------------------------------------------------------------

// shown loads the first page when the tab is first looked at.
func (t *releasesTab) shown(m *Model) tea.Cmd {
	if t.loaded {
		return nil
	}
	return t.load(m, true)
}

// moved loads the next page when the cursor nears the end of the list.
func (t *releasesTab) moved(m *Model) tea.Cmd {
	if t.nextOffset > 0 && t.Cursor() >= t.Len()-3 {
		return t.load(m, false)
	}
	return nil
}

// filtered goes on loading pages while a filter is typed.
func (t *releasesTab) filtered(m *Model) tea.Cmd { return t.search(m) }

func (t *releasesTab) loading() bool { return t.busy }

// status names the stream listed, on the right of the filter row.
func (t *releasesTab) status() string {
	label, _ := unitycli.StreamLabel(t.stream)
	return label
}

// ---- loading -----------------------------------------------------------------------

// load asks for a page of releases. With reset the list starts over.
func (t *releasesTab) load(m *Model, reset bool) tea.Cmd {
	if t.busy {
		return nil
	}
	offset := t.nextOffset
	if reset {
		offset = 0
	}
	stream, gen := t.stream, t.gen
	limit := releasesPerPage
	if t.filter() != "" {
		limit = releasesPerSearch
	}
	page := m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		p, err := c.Releases(ctx, stream, offset, limit)
		return releasesMsg{stream: stream, gen: gen, offset: offset, page: p, err: err}
	})
	if page == nil {
		return nil
	}
	t.busy = true
	return tea.Batch(page, m.startSpinner())
}

// search loads the next page while a filter is set, so that the filter looks
// through every release and not only the pages loaded.
func (t *releasesTab) search(m *Model) tea.Cmd {
	if t.filter() == "" || !t.loaded || t.nextOffset == 0 {
		return nil
	}
	return t.load(m, false)
}

// restart empties the list and loads it again, from a stream.
func (t *releasesTab) restart(m *Model, stream string) tea.Cmd {
	t.stream, t.gen = stream, t.gen+1
	t.nextOffset, t.loaded, t.busy = 0, false, false
	t.SetItems(nil)
	t.empty = "Loading releases…"
	return t.load(m, true)
}

// refresh loads the list again if it was loaded, keeping what is shown until
// the first page is back.
func (t *releasesTab) refresh(m *Model) tea.Cmd {
	if !t.loaded {
		return nil
	}
	t.gen++
	t.nextOffset, t.loaded, t.busy = 0, false, false
	return t.load(m, true)
}

// onPage takes in a page.
func (t *releasesTab) onPage(m *Model, msg releasesMsg) tea.Cmd {
	if msg.stream != t.stream || msg.gen != t.gen {
		return nil // the list started over while this was loading
	}
	t.busy = false
	if msg.err != nil {
		m.toastf(toastError, "Could not list releases: %v", msg.err)
		t.empty = "Could not load releases. Press R to retry."
		return nil
	}
	items := msg.page.Items
	if msg.offset > 0 {
		items = append(append([]unitycli.Release(nil), t.Items()...), items...)
	}
	t.loaded = true
	t.nextOffset = msg.page.NextOffset
	t.empty = "No releases."
	t.SetItems(items)
	return t.search(m)
}

// ---- actions --------------------------------------------------------------------

func releaseActions() []*action {
	on := onSelected(func(m *Model) (unitycli.Release, bool) { return m.releasesTab.Selected() })
	return []*action{
		on(&action{id: "releases.install", ctx: ctxReleases, keys: []string{"enter", "space"}, desc: "Install this version", hint: "Install"},
			func(m *Model, r unitycli.Release) tea.Cmd {
				if r.Installed {
					m.toastf(toastInfo, "Unity %s is already installed", r.Version)
					return nil
				}
				return m.installEditorFlow(r.Version, "")
			}),
		{id: "releases.stream", ctx: ctxReleases, keys: []string{"ctrl+s"}, desc: "Filter by release stream (Supported, LTS, Beta, Alpha)", hint: "Stream",
			run: func(m *Model) tea.Cmd { m.streamMenu(); return nil }},
		on(&action{id: "releases.notes", ctx: ctxReleases, keys: []string{"o"}, desc: "Open the release notes in the browser"},
			func(m *Model, r unitycli.Release) tea.Cmd { return m.openURL(releaseNotesURL(r.Version)) }),
	}
}

// releaseNotesURL is the page of a release's notes.
func releaseNotesURL(version string) string {
	return "https://unity.com/releases/editor/whats-new/" + version
}

func (m *Model) streamMenu() {
	var choices []choice
	for _, s := range unitycli.ReleaseStreams {
		choices = append(choices, choice{s.ID, s.Label, s.Desc})
	}
	m.choiceMenu("Release stream", choices, m.releasesTab.stream, func(m *Model, stream string) tea.Cmd {
		m.st.SetStream(stream)
		m.saveState()
		return m.releasesTab.restart(m, stream)
	})
}

// ---- drawing ----------------------------------------------------------------------

// streamLabel names a release's stream for display and says how to colour it.
func (th *Theme) streamLabel(r unitycli.Release) (string, lipgloss.Style) {
	label, known := "", false
	if r.Stream != "" {
		label, known = unitycli.StreamLabel(r.Stream)
	}
	if !known && r.LTS {
		label = "LTS"
	}
	switch label {
	case "LTS":
		return label, th.Success
	case "Beta", "Alpha":
		return label, th.Warning
	}
	return label, th.Dim
}

func (m *Model) drawRelease(r unitycli.Release, w int, base lipgloss.Style) string {
	th := m.th
	row := newRow(base)
	row.add(r.Version, th.Text, 14).gap(1)
	label, st := th.streamLabel(r)
	row.add(label, st, 9).gap(1)
	row.add(r.Architecture, th.Dim, 6)
	if r.Installed {
		row.gap(1).text("✓ installed", th.Success)
	}
	return row.done(w)
}
