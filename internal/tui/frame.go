package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// scrollbar describes a list's scroll position for the thumb on a frame's edge.
type scrollbar struct{ offset, visible, total int }

// frame is one bordered panel, drawn by hand because lipgloss cannot put a
// title inside a border the way lazygit does.
type frame struct {
	w, h      int
	jump      string   // the number in [1]; empty for none
	tabs      []string // title, or the tab names
	tab       int      // the current tab
	focused   bool
	searching bool     // a filter is being typed: use the searching colour
	lines     []string // content, at most h-2 lines
	footer    string   // "3 of 20", bottom right
	scroll    scrollbar
}

// renderFrame draws the frame: exactly h lines of exactly w cells.
func (t *Theme) renderFrame(f frame) string {
	if f.w < 3 || f.h < 2 {
		return ""
	}
	b := t.Border
	bs := t.InactiveBorder
	switch {
	case f.searching:
		bs = t.SearchingBorder
	case f.focused:
		bs = t.ActiveBorder
	}
	inner := f.w - 2

	// ---- top border: ╭─[1]─Tab - Tab────╮
	var title strings.Builder
	if f.jump != "" {
		title.WriteString(bs.Render("[" + f.jump + "]"))
		title.WriteString(bs.Render(b.h))
	}
	// the current tab keeps its colour when the panel is not focused, as in lazygit
	current := t.ActiveBorder.Bold(false)
	if f.focused || f.searching {
		current = bs
	}
	for i, name := range f.tabs {
		if i > 0 {
			title.WriteString(t.Text.Render(" - "))
		}
		if i == f.tab {
			title.WriteString(current.Render(name))
		} else {
			title.WriteString(t.Text.Render(name))
		}
	}
	ts := truncate(title.String(), f.w-3)
	fill := max(f.w-3-width(ts), 0)
	top := bs.Render(b.tl+b.h) + ts + bs.Render(strings.Repeat(b.h, fill)+b.tr)

	// ---- bottom border: ╰────── 3 of 20─╯
	footer := f.footer
	if width(footer) > f.w-3 {
		footer = ""
	}
	var bottom string
	if footer == "" {
		bottom = bs.Render(b.bl + strings.Repeat(b.h, f.w-2) + b.br)
	} else {
		fill := f.w - 3 - width(footer)
		bottom = bs.Render(b.bl+strings.Repeat(b.h, fill)) + t.Text.Render(footer) + bs.Render(b.h+b.br)
	}

	// ---- sides, with the scrollbar thumb on the right edge
	rows := f.h - 2
	thumbTop, thumbLen := -1, 0
	if sb := f.scroll; sb.total > sb.visible && sb.visible > 0 && rows > 0 {
		thumbLen = max(1, rows*sb.visible/sb.total)
		span := sb.total - sb.visible
		thumbTop = (rows - thumbLen) * clamp(sb.offset, 0, span) / span
	}
	left := bs.Render(b.v)
	var sb strings.Builder
	sb.WriteString(top)
	for i := 0; i < rows; i++ {
		line := ""
		if i < len(f.lines) {
			line = f.lines[i]
		}
		right := bs.Render(b.v)
		if thumbTop >= 0 && i >= thumbTop && i < thumbTop+thumbLen {
			right = bs.Render("▐")
		}
		sb.WriteByte('\n')
		sb.WriteString(left + fit(line, inner) + right)
	}
	sb.WriteByte('\n')
	sb.WriteString(bottom)
	return sb.String()
}

// footerText is the "3 of 20" shown on a list's bottom border.
func footerText(cursor, total int) string {
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("%d of %d", cursor+1, total)
}

// joinColumns puts blocks side by side.
func joinColumns(blocks ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
}
