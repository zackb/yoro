package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

// Column renders a titled, bordered column of fixed outer width/height. body is
// the inner content (already styled); it is clipped to the inner area.
func (t Theme) Column(title, body string, width, height int, focused bool) string {
	style := t.ColumnStyle(focused)
	innerW := width - 2  // borders
	innerH := height - 2 // borders
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}

	head := t.ColTitle.Render(Truncate(title, innerW))
	lines := strings.Split(body, "\n")
	// Reserve one line for the header.
	avail := innerH - 1
	if avail < 0 {
		avail = 0
	}
	if len(lines) > avail {
		lines = lines[:avail]
	}
	for len(lines) < avail {
		lines = append(lines, "")
	}
	content := head + "\n" + strings.Join(lines, "\n")
	return style.Width(innerW).Height(innerH).Render(content)
}

// ColumnStyle returns the bordered column style for the given focus state.
func (t Theme) ColumnStyle(focused bool) lipgloss.Style {
	if focused {
		return t.boxFocus
	}
	return t.boxStyle
}

// Truncate clips s to a display width of w, appending an ellipsis when cut.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return truncate.StringWithTail(s, uint(w), "…")
}

// PadRight pads s with spaces to a display width of w (no truncation).
func PadRight(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

// scrollWindow returns the slice of lines visible when cursor must remain in
// view within a window of the given height, plus the index of the first line.
func scrollWindow(lines []string, cursor, height int) ([]string, int) {
	if height <= 0 || len(lines) == 0 {
		return nil, 0
	}
	top := 0
	if cursor >= height {
		top = cursor - height + 1
	}
	if top > len(lines)-height {
		top = len(lines) - height
	}
	if top < 0 {
		top = 0
	}
	end := top + height
	if end > len(lines) {
		end = len(lines)
	}
	return lines[top:end], top
}

// oneLine collapses all whitespace (including line breaks) to single spaces, for
// values shown on a single row (agenda entries, titles).
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// clamp constrains v to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
