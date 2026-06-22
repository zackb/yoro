package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/zackb/yoro/internal/model"
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

// TestSyncAnchorOutOfBoundsCursor guards the defensive bounds check: a stale
// curIdx (left over after a rebuild dropped rows) must not panic syncAnchor.
func TestSyncAnchorOutOfBoundsCursor(t *testing.T) {
	p := &calendarPane{
		focus:   focusMiddle,
		rows:    []agendaRow{{day: time.Now()}},
		selRows: []int{0},
		curIdx:  5, // stale, past the end of selRows
	}
	p.syncAnchor() // must not panic
}

// TestMonthGridNavigation verifies the day cursor moves by day/week/month and
// that the shown month (anchor) always tracks the cursor's month. The window is
// kept wide so ensureWindow never touches the (nil) store.
func TestMonthGridNavigation(t *testing.T) {
	now := dayStart(time.Now())
	p := &calendarPane{
		view:    viewMonth,
		gridDay: now,
		anchor:  startOfMonth(now),
		window:  model.DateRange{From: now.AddDate(-1, 0, 0), To: now.AddDate(1, 0, 0)},
		enabled: map[string]bool{},
	}

	p.moveGrid(0, 0, 1)
	if !sameDay(p.gridDay, now.AddDate(0, 0, 1)) {
		t.Errorf("day move: gridDay = %v, want %v", p.gridDay, now.AddDate(0, 0, 1))
	}
	p.moveGrid(0, 0, 7)
	if !sameDay(p.gridDay, now.AddDate(0, 0, 8)) {
		t.Errorf("week move: gridDay = %v, want %v", p.gridDay, now.AddDate(0, 0, 8))
	}
	p.moveGrid(0, 1, 0)
	if !p.anchor.Equal(startOfMonth(p.gridDay)) {
		t.Errorf("anchor %v does not track cursor month %v", p.anchor, startOfMonth(p.gridDay))
	}
}

// TestToggleMonthViewRoundTrip verifies the selection carries across the toggle:
// entering the grid seeds the day from the agenda cursor; leaving it drops the
// agenda cursor onto the first occurrence on/after that day.
func TestToggleMonthViewRoundTrip(t *testing.T) {
	d1 := dayStart(time.Date(2026, 6, 10, 0, 0, 0, 0, time.Local))
	d2 := d1.AddDate(0, 0, 3)
	p := &calendarPane{
		view: viewAgenda,
		rows: []agendaRow{
			{header: true, day: d1},
			{day: d1, occ: model.Occurrence{Start: d1.Add(9 * time.Hour)}},
			{header: true, day: d2},
			{day: d2, occ: model.Occurrence{Start: d2.Add(9 * time.Hour)}},
		},
		selRows: []int{1, 3},
		curIdx:  1, // on the d2 occurrence
	}

	p.toggleMonthView()
	if p.view != viewMonth {
		t.Fatalf("expected viewMonth after toggle")
	}
	if !sameDay(p.gridDay, d2) {
		t.Errorf("gridDay seeded to %v, want %v", p.gridDay, d2)
	}

	p.gridDay = d1 // move the cursor back to the first day
	p.toggleMonthView()
	if p.view != viewAgenda {
		t.Fatalf("expected viewAgenda after second toggle")
	}
	if p.curIdx != 0 {
		t.Errorf("agenda cursor = %d, want 0 (first occ on/after %v)", p.curIdx, d1)
	}
}
