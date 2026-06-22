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

// TestPageMonthClampsDay verifies whole-month paging keeps the cursor on the same
// day-of-month, clamped to the target month's length (Jan 31 -> Feb 28, not Mar 3),
// and always lands the anchor on the target month.
func TestPageMonthClampsDay(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	}
	wide := model.DateRange{From: day(2025, 1, 1), To: day(2027, 1, 1)}
	cases := []struct {
		name string
		from time.Time
		dir  int
		want time.Time
	}{
		{"jan31 forward clamps to feb28", day(2026, 1, 31), 1, day(2026, 2, 28)},
		{"mar31 back clamps to feb28", day(2026, 3, 31), -1, day(2026, 2, 28)},
		{"mid-month forward keeps day", day(2026, 6, 15), 1, day(2026, 7, 15)},
		{"dec back rolls year", day(2026, 1, 10), -1, day(2025, 12, 10)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &calendarPane{view: viewMonth, gridDay: c.from, anchor: startOfMonth(c.from), window: wide, enabled: map[string]bool{}}
			p.pageMonth(c.dir)
			if !sameDay(p.gridDay, c.want) {
				t.Errorf("gridDay = %v, want %v", p.gridDay, c.want)
			}
			if !p.anchor.Equal(startOfMonth(c.want)) {
				t.Errorf("anchor = %v, want %v", p.anchor, startOfMonth(c.want))
			}
		})
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
