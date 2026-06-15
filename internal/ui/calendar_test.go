package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestWrapLinesPreservesBreaksAndWraps verifies a multi-line location both keeps
// its author line break and word-wraps long lines to width (no ellipsis), which
// is what the event detail pane relies on.
func TestWrapLinesPreservesBreaksAndWraps(t *testing.T) {
	const width = 30
	loc := "Newmark Theatre\n1111 SW Broadway, Portland, OR  97205, United States"

	lines := wrapLines(loc, width)
	if len(lines) < 3 {
		t.Fatalf("expected the long line to wrap into extra rows, got %d: %q", len(lines), lines)
	}
	if lines[0] != "Newmark Theatre" {
		t.Errorf("author line break not preserved; first row = %q", lines[0])
	}
	for _, l := range lines {
		if lipgloss.Width(l) > width {
			t.Errorf("row exceeds width %d: %q (%d)", width, l, lipgloss.Width(l))
		}
		if strings.Contains(l, "…") {
			t.Errorf("row was truncated with an ellipsis instead of wrapped: %q", l)
		}
	}
}
