package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"

	"github.com/zackb/yoro/internal/model"
	"github.com/zackb/yoro/internal/store"
)

// agendaRow is one rendered line: either a day header or a selectable occurrence.
type agendaRow struct {
	header bool
	day    time.Time
	occ    model.Occurrence
}

// calView selects the calendar pane's sub-view: the agenda list or the full
// month grid.
type calView int

const (
	viewAgenda calView = iota
	viewMonth
)

// calendarPane is the agenda + mini-month + detail view.
type calendarPane struct {
	theme Theme
	keys  KeyMap
	store store.Store

	width, height int

	cals        []model.Collection
	colByID     map[string]model.Collection
	enabled     map[string]bool
	sidebarIdx  int
	srcType     map[string]string // source id -> type, for provenance glyphs
	multiSource bool

	window  model.DateRange
	rows    []agendaRow
	selRows []int     // indices into rows that are selectable occurrences
	curIdx  int       // index into selRows
	anchor  time.Time // month shown in the mini-month

	showTasks bool
	focus     focusCol
	status    string

	view    calView   // agenda list vs full month grid
	gridDay time.Time // selected day cursor in month-grid view

	searching bool            // are we in search-input mode?
	search    textinput.Model // the search text input
	query     string          // committed/live search query
}

func newCalendarPane(theme Theme, keys KeyMap, st store.Store) *calendarPane {
	now := time.Now()
	ti := textinput.New()
	ti.Prompt = IconSearch + " "
	ti.Placeholder = "search events"
	return &calendarPane{
		theme:   theme,
		keys:    keys,
		store:   st,
		enabled: map[string]bool{},
		anchor:  startOfMonth(now),
		window:  model.DateRange{From: dayStart(now).AddDate(0, 0, -7), To: dayStart(now).AddDate(0, 0, 56)},
		focus:   focusMiddle,
		search:  ti,
	}
}

func (p *calendarPane) refresh() {
	p.cals = p.store.Calendars()
	srcs := p.store.Sources()
	p.multiSource = len(srcs) > 1
	p.srcType = map[string]string{}
	for _, s := range srcs {
		p.srcType[s.ID] = s.Type
	}
	p.colByID = map[string]model.Collection{}
	for _, c := range p.cals {
		p.colByID[c.ID] = c
		if _, ok := p.enabled[c.ID]; !ok {
			p.enabled[c.ID] = true
		}
	}
	p.rebuild()
	p.jumpToday()
}

func (p *calendarPane) setSize(w, h int) { p.width, p.height = w, h }

// rebuild recomputes occurrences and agenda rows for the current window.
func (p *calendarPane) rebuild() {
	occs := p.filterOccs(p.store.Occurrences(p.window, p.enabled))
	p.rows = p.rows[:0]
	p.selRows = p.selRows[:0]

	// Bucket each occurrence under every day it spans so multi-day events appear
	// under each day's header, not just their start day. occs is start-sorted, so
	// appending in order keeps each day's entries ordered by start.
	byDay := map[string][]model.Occurrence{}
	dayOf := map[string]time.Time{}
	winStart := dayStart(p.window.From)
	for _, o := range occs {
		for _, d := range o.Days() {
			// A boundary-crossing event spans days outside the display window; skip
			// those so they don't emit stray headers before/after the window.
			if d.Before(winStart) || !d.Before(p.window.To) {
				continue
			}
			k := d.Format("2006-01-02")
			byDay[k] = append(byDay[k], o)
			dayOf[k] = d
		}
	}
	keys := make([]string, 0, len(byDay))
	for k := range byDay {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	add := func(day time.Time, occ model.Occurrence, header bool) {
		p.rows = append(p.rows, agendaRow{header: header, day: day, occ: occ})
		if !header {
			p.selRows = append(p.selRows, len(p.rows)-1)
		}
	}
	for _, k := range keys {
		d := dayOf[k]
		add(d, model.Occurrence{}, true)
		for _, o := range byDay[k] {
			add(d, o, false)
		}
	}
	p.curIdx = clamp(p.curIdx, 0, max0(len(p.selRows)-1))
}

// filterOccs narrows occurrences to those matching the live search query. With no
// query it is a pass-through, so all occurrence reads (agenda, month grid,
// mini-month) flow through it uniformly.
func (p *calendarPane) filterOccs(occs []model.Occurrence) []model.Occurrence {
	q := strings.ToLower(strings.TrimSpace(p.query))
	if q == "" {
		return occs
	}
	out := occs[:0:0]
	for _, o := range occs {
		if occContains(o, q) {
			out = append(out, o)
		}
	}
	return out
}

// occContains reports whether an occurrence matches a lowercased query across its
// summary, description, location, and attendees.
func occContains(o model.Occurrence, q string) bool {
	if strings.Contains(strings.ToLower(o.Summary), q) {
		return true
	}
	if o.Event != nil {
		if strings.Contains(strings.ToLower(o.Event.Description), q) ||
			strings.Contains(strings.ToLower(o.Event.Location), q) {
			return true
		}
		for _, a := range o.Event.Attendees {
			if strings.Contains(strings.ToLower(a.Name), q) ||
				strings.Contains(strings.ToLower(a.Email), q) {
				return true
			}
		}
	}
	return false
}

func (p *calendarPane) Update(msg tea.Msg) (tea.Cmd, bool) {
	if p.searching {
		return p.updateSearch(msg)
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	if key.Matches(km, p.keys.Month) {
		p.toggleMonthView()
		return nil, true
	}
	if p.view == viewMonth {
		return p.updateMonth(km)
	}
	switch {
	case key.Matches(km, p.keys.Left):
		if p.focus > focusLeft {
			p.focus--
		}
	case key.Matches(km, p.keys.Right):
		if p.focus < focusRight {
			p.focus++
		}
	case key.Matches(km, p.keys.Down):
		p.moveDown(1)
	case key.Matches(km, p.keys.Up):
		p.moveUp(1)
	case key.Matches(km, p.keys.HalfDown):
		p.moveDown(max0(p.height / 2))
	case key.Matches(km, p.keys.HalfUp):
		p.moveUp(max0(p.height / 2))
	case key.Matches(km, p.keys.PageDown):
		p.moveDown(max0(p.height))
	case key.Matches(km, p.keys.PageUp):
		p.moveUp(max0(p.height))
	case key.Matches(km, p.keys.Top):
		if km.String() == "g" {
			p.cursorTo(0)
		}
	case key.Matches(km, p.keys.Bottom):
		p.cursorTo(len(p.selRows) - 1)
	case key.Matches(km, p.keys.Today):
		p.jumpToday()
	case key.Matches(km, p.keys.NextDay):
		p.jumpDay(1)
	case key.Matches(km, p.keys.PrevDay):
		p.jumpDay(-1)
	case key.Matches(km, p.keys.NextMonth):
		p.shiftMonth(1)
	case key.Matches(km, p.keys.PrevMonth):
		p.shiftMonth(-1)
	case key.Matches(km, p.keys.Toggle):
		p.toggleCollection()
	case key.Matches(km, p.keys.Tasks):
		p.showTasks = !p.showTasks
		p.rebuild()
	case key.Matches(km, p.keys.Search):
		p.startSearch()
	case key.Matches(km, p.keys.Escape):
		if p.query == "" {
			return nil, false
		}
		p.query = ""
		p.rebuild()
	default:
		return nil, false
	}
	p.syncAnchor()
	return nil, true
}

func (p *calendarPane) startSearch() {
	p.searching = true
	p.view = viewAgenda // search always operates on the agenda list
	p.focus = focusMiddle
	p.search.SetValue(p.query)
	p.search.CursorEnd()
	p.search.Focus()
}

func (p *calendarPane) updateSearch(msg tea.Msg) (tea.Cmd, bool) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "enter":
			p.query = p.search.Value()
			p.searching = false
			p.search.Blur()
			p.rebuild()
			return nil, true
		case "esc":
			p.searching = false
			p.search.Blur()
			return nil, true
		}
	}
	var cmd tea.Cmd
	p.search, cmd = p.search.Update(msg)
	p.query = p.search.Value()
	p.rebuild()
	return cmd, true
}

// toggleMonthView flips between the agenda list and the month grid, carrying the
// selection across: entering the grid seeds the day cursor from the selected
// occurrence (or today); leaving it drops the agenda cursor onto that day.
func (p *calendarPane) toggleMonthView() {
	if p.view == viewAgenda {
		p.gridDay = dayStart(time.Now())
		if o, ok := p.selectedOcc(); ok {
			p.gridDay = o.Day()
		}
		p.anchor = startOfMonth(p.gridDay)
		p.view = viewMonth
		return
	}
	p.view = viewAgenda
	p.focus = focusMiddle
	p.cursorToDay(p.gridDay)
}

// cursorToDay places the agenda cursor on the first occurrence on/after day.
func (p *calendarPane) cursorToDay(day time.Time) {
	for i, ri := range p.selRows {
		if !p.rows[ri].day.Before(day) {
			p.curIdx = i
			return
		}
	}
	p.curIdx = max0(len(p.selRows) - 1)
}

// updateMonth handles keys while the month grid is shown. The cursor is a day:
// h/l move ±1 day, j/k move ±1 week, J/K and t jump months/today, and enter
// drills back into the agenda list at the selected day.
func (p *calendarPane) updateMonth(km tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case km.String() == "enter":
		p.toggleMonthView()
	case key.Matches(km, p.keys.Left):
		p.moveGrid(0, 0, -1)
	case key.Matches(km, p.keys.Right):
		p.moveGrid(0, 0, 1)
	case key.Matches(km, p.keys.Up):
		p.moveGrid(0, 0, -7)
	case key.Matches(km, p.keys.Down):
		p.moveGrid(0, 0, 7)
	case key.Matches(km, p.keys.NextMonth):
		p.moveGrid(0, 1, 0)
	case key.Matches(km, p.keys.PrevMonth):
		p.moveGrid(0, -1, 0)
	case key.Matches(km, p.keys.Today):
		p.gridDay = dayStart(time.Now())
		p.anchor = startOfMonth(p.gridDay)
		p.ensureWindow(p.gridDay)
	default:
		return nil, false
	}
	return nil, true
}

// moveGrid shifts the day cursor by the given years/months/days, keeps the shown
// month in step, and extends the loaded window to cover the new day.
func (p *calendarPane) moveGrid(years, months, days int) {
	p.gridDay = p.gridDay.AddDate(years, months, days)
	p.anchor = startOfMonth(p.gridDay)
	p.ensureWindow(p.gridDay)
}

func (p *calendarPane) moveDown(n int) {
	if p.focus == focusLeft {
		p.sidebarIdx = clamp(p.sidebarIdx+n, 0, max0(len(p.cals)-1))
		return
	}
	if p.curIdx >= len(p.selRows)-1 {
		p.extendForward()
	}
	p.cursorTo(p.curIdx + n)
}

func (p *calendarPane) moveUp(n int) {
	if p.focus == focusLeft {
		p.sidebarIdx = clamp(p.sidebarIdx-n, 0, max0(len(p.cals)-1))
		return
	}
	if p.curIdx == 0 {
		p.extendBackward()
	}
	p.cursorTo(p.curIdx - n)
}

func (p *calendarPane) cursorTo(i int) { p.curIdx = clamp(i, 0, max0(len(p.selRows)-1)) }

func (p *calendarPane) toggleCollection() {
	if p.focus != focusLeft || p.sidebarIdx >= len(p.cals) {
		return
	}
	id := p.cals[p.sidebarIdx].ID
	p.enabled[id] = !p.enabled[id]
	p.rebuild()
}

// jumpToday moves the cursor to the first occurrence on/after today.
func (p *calendarPane) jumpToday() {
	today := dayStart(time.Now())
	p.anchor = startOfMonth(today)
	for i, ri := range p.selRows {
		if !p.rows[ri].day.Before(today) {
			p.curIdx = i
			return
		}
	}
	p.curIdx = max0(len(p.selRows) - 1)
}

// jumpDay moves the cursor to the first occurrence of the next/previous day
// that has events.
func (p *calendarPane) jumpDay(dir int) {
	if len(p.selRows) == 0 {
		return
	}
	curDay := p.rows[p.selRows[p.curIdx]].day
	if dir > 0 {
		for i := p.curIdx + 1; i < len(p.selRows); i++ {
			if p.rows[p.selRows[i]].day.After(curDay) {
				p.curIdx = i
				return
			}
		}
		p.extendForward()
	} else {
		for i := p.curIdx - 1; i >= 0; i-- {
			if p.rows[p.selRows[i]].day.Before(curDay) {
				// move to the first occurrence of that earlier day
				target := p.rows[p.selRows[i]].day
				for j := i; j >= 0; j-- {
					if p.rows[p.selRows[j]].day.Before(target) {
						p.curIdx = j + 1
						return
					}
				}
				p.curIdx = 0
				return
			}
		}
		p.extendBackward()
	}
}

func (p *calendarPane) shiftMonth(dir int) {
	p.anchor = p.anchor.AddDate(0, dir, 0)
	// Ensure the window covers the new month, then jump the agenda into it.
	p.ensureWindow(p.anchor)
	p.ensureWindow(endOfMonth(p.anchor))
	target := p.anchor
	for i, ri := range p.selRows {
		if !p.rows[ri].day.Before(target) {
			p.curIdx = i
			return
		}
	}
}

// syncAnchor keeps the mini-month in step with the cursor's day.
func (p *calendarPane) syncAnchor() {
	if p.focus == focusLeft || p.curIdx < 0 || p.curIdx >= len(p.selRows) {
		return
	}
	p.anchor = startOfMonth(p.rows[p.selRows[p.curIdx]].day)
}

func (p *calendarPane) extendForward() {
	p.window.To = p.window.To.AddDate(0, 0, 56)
	cur := p.curIdx
	p.rebuild()
	// Forward extension appends rows, so the cursor's row is unchanged; clamp
	// defensively in case rebuild dropped rows (e.g. a collection toggled off).
	p.curIdx = clamp(cur, 0, max0(len(p.selRows)-1))
}

func (p *calendarPane) extendBackward() {
	prevLen := len(p.selRows)
	p.window.From = p.window.From.AddDate(0, 0, -56)
	p.rebuild()
	// keep cursor on the same occurrence by offsetting by how many were prepended
	p.curIdx += len(p.selRows) - prevLen
	p.curIdx = clamp(p.curIdx, 0, max0(len(p.selRows)-1))
}

func (p *calendarPane) ensureWindow(day time.Time) {
	if day.Before(p.window.From) {
		p.window.From = startOfMonth(day).AddDate(0, 0, -7)
		p.rebuild()
	}
	if !day.Before(p.window.To) {
		p.window.To = endOfMonth(day).AddDate(0, 0, 7)
		p.rebuild()
	}
}

// selectedCalendar returns the sidebar-highlighted calendar, the target for a
// new event.
func (p *calendarPane) selectedCalendar() (model.Collection, bool) {
	if p.sidebarIdx < 0 || p.sidebarIdx >= len(p.cals) {
		return model.Collection{}, false
	}
	return p.cals[p.sidebarIdx], true
}

func (p *calendarPane) selectedOcc() (model.Occurrence, bool) {
	if p.curIdx < 0 || p.curIdx >= len(p.selRows) {
		return model.Occurrence{}, false
	}
	return p.rows[p.selRows[p.curIdx]].occ, true
}

// ---- rendering ----

func (p *calendarPane) View() string {
	if p.view == viewMonth {
		return p.monthView()
	}
	w, h := p.width, p.height
	sideW, agendaW, detailW := threeColumns(w, 26, 18, 34, 26, 46)

	agendaTitle := "AGENDA"
	if p.query != "" {
		agendaTitle = fmt.Sprintf("AGENDA (%d)", len(p.selRows))
	}
	side := p.theme.Column("CALENDARS", p.sidebarBody(sideW-2, h-3), sideW, h, p.focus == focusLeft)
	agenda := p.theme.Column(agendaTitle, p.agendaBody(agendaW-2, h-3), agendaW, h, p.focus == focusMiddle)
	detail := p.theme.Column("EVENT", p.detailBody(detailW-2), detailW, h, p.focus == focusRight)
	return lipgloss.JoinHorizontal(lipgloss.Top, side, agenda, detail)
}

// monthView renders the full-width month grid alongside the detail column for the
// selected day's events.
func (p *calendarPane) monthView() string {
	w, h := p.width, p.height
	detailW := clamp(w*30/100, 26, 40)
	gridW := max0(w - detailW)

	grid := p.theme.Column(strings.ToUpper(p.anchor.Format("January 2006")),
		p.monthGridBody(gridW-2, h-3), gridW, h, true)
	detail := p.theme.Column(p.gridDay.Format("Mon, Jan 2"),
		p.monthDetailBody(detailW-2), detailW, h, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, grid, detail)
}

// monthGridBody draws a Monday-based 6-week grid with box-drawing borders around
// every day cell. Each cell shows event titles when wide enough, otherwise falls
// back to colored dots and a +N overflow count.
func (p *calendarPane) monthGridBody(w, h int) string {
	// Reserve a column for the 8 vertical rules (left edge + 6 inner + right edge),
	// then split the rest into 7 cells, handing the remainder to the leftmost cells
	// so the grid fills the full width.
	inner := w - 8
	if inner/7 < 3 {
		return p.theme.ItemDim.Render("window too narrow")
	}
	widths := make([]int, 7)
	for i := range widths {
		widths[i] = inner / 7
		if i < inner%7 {
			widths[i]++
		}
	}
	// Body height minus the border/separator rows (top, header, header rule, 5
	// inner rules, bottom = 9), split across 6 weeks.
	weekH := max0((h - 9) / 6)
	if weekH < 1 {
		weekH = 1
	}

	today := dayStart(time.Now())
	byDay := p.occurrencesByDay(p.anchor)

	border := lipgloss.NewStyle().Foreground(p.theme.Border)
	v := border.Render("│")
	hline := func(l, m, r string) string {
		parts := []string{l}
		for i := 0; i < 7; i++ {
			if i > 0 {
				parts = append(parts, m)
			}
			parts = append(parts, strings.Repeat("─", widths[i]))
		}
		parts = append(parts, r)
		return border.Render(strings.Join(parts, ""))
	}
	joinRow := func(cells []string) string { return v + strings.Join(cells, v) + v }

	var b strings.Builder
	b.WriteString(hline("┌", "┬", "┐") + "\n")

	names := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	hdr := make([]string, 7)
	for i, n := range names {
		hdr[i] = p.theme.Label.Render(PadRight(Truncate(n, widths[i]), widths[i]))
	}
	b.WriteString(joinRow(hdr) + "\n")
	b.WriteString(hline("├", "┼", "┤") + "\n")

	first := startOfMonth(p.anchor)
	offset := (int(first.Weekday()) + 6) % 7 // Monday-based
	day := first.AddDate(0, 0, -offset)
	for week := 0; week < 6; week++ {
		cells := make([][]string, 7)
		for d := 0; d < 7; d++ {
			cells[d] = p.gridCell(day, today, byDay, widths[d], weekH)
			day = day.AddDate(0, 0, 1)
		}
		for r := 0; r < weekH; r++ {
			row := make([]string, 7)
			for d := 0; d < 7; d++ {
				row[d] = cells[d][r]
			}
			b.WriteString(joinRow(row) + "\n")
		}
		if week < 5 {
			b.WriteString(hline("├", "┼", "┤") + "\n")
		}
	}
	b.WriteString(hline("└", "┴", "┘"))
	return b.String()
}

// gridCell renders one day as the weekH content lines of a cellW-wide block.
func (p *calendarPane) gridCell(day, today time.Time, byDay map[string][]model.Occurrence, cellW, weekH int) []string {
	otherMonth := day.Month() != p.anchor.Month()
	num := fmt.Sprintf("%2d", day.Day())
	switch {
	case sameDay(day, p.gridDay):
		num = p.theme.Today.Render(num)
	case sameDay(day, today):
		num = p.theme.DayHeader.Render(num)
	case otherMonth:
		num = p.theme.ItemDim.Render(num)
	default:
		num = p.theme.Value.Render(num)
	}

	lines := make([]string, 0, weekH)
	lines = append(lines, PadRight(num, cellW))

	occs := byDay[day.Format("2006-01-02")]
	bodyRows := weekH - 1
	if len(occs) > 0 && bodyRows > 0 {
		if cellW >= 8 {
			lines = append(lines, p.cellTitles(occs, cellW, bodyRows, otherMonth)...)
		} else {
			lines = append(lines, p.cellDots(occs, cellW))
		}
	}
	for len(lines) < weekH {
		lines = append(lines, strings.Repeat(" ", cellW))
	}
	return lines[:weekH]
}

// cellTitles renders up to bodyRows event summaries, with a "+N more" final row
// when there are more events than fit.
func (p *calendarPane) cellTitles(occs []model.Occurrence, cellW, bodyRows int, dim bool) []string {
	out := make([]string, 0, bodyRows)
	shown := len(occs)
	if shown > bodyRows {
		shown = max0(bodyRows - 1) // leave a row for the overflow count
	}
	for i := 0; i < shown; i++ {
		title := fmt.Sprintf("%s %s", colorDot(occs[i].Color), oneLine(occs[i].Summary))
		style := p.theme.Value
		if dim {
			style = p.theme.ItemDim
		}
		out = append(out, style.Render(PadRight(Truncate(title, cellW), cellW)))
	}
	if rest := len(occs) - shown; rest > 0 {
		out = append(out, p.theme.ItemDim.Render(PadRight(Truncate(fmt.Sprintf("+%d more", rest), cellW), cellW)))
	}
	return out
}

// cellDots renders a compact dots + overflow line for narrow cells.
func (p *calendarPane) cellDots(occs []model.Occurrence, cellW int) string {
	maxDots := max0((cellW - 1) / 2)
	if maxDots < 1 {
		maxDots = 1
	}
	parts := make([]string, 0, maxDots+1)
	for i := 0; i < len(occs) && i < maxDots; i++ {
		parts = append(parts, colorDot(occs[i].Color))
	}
	line := strings.Join(parts, " ")
	if rest := len(occs) - maxDots; rest > 0 {
		line += fmt.Sprintf("+%d", rest)
	}
	return PadRight(Truncate(line, cellW), cellW)
}

// monthDetailBody lists the selected day's events in the detail column.
func (p *calendarPane) monthDetailBody(w int) string {
	occs := p.occurrencesByDay(p.anchor)[p.gridDay.Format("2006-01-02")]
	if len(occs) == 0 {
		return p.theme.ItemDim.Render("no events")
	}
	var b strings.Builder
	for _, o := range occs {
		b.WriteString(p.theme.Value.Render(p.occRow(o, w)) + "\n")
	}
	return b.String()
}

// occurrencesByDay buckets the visible 6-week window's occurrences by day key.
func (p *calendarPane) occurrencesByDay(anchor time.Time) map[string][]model.Occurrence {
	out := map[string][]model.Occurrence{}
	win := model.DateRange{From: startOfMonth(anchor).AddDate(0, 0, -7), To: endOfMonth(anchor).AddDate(0, 0, 7)}
	for _, o := range p.filterOccs(p.store.Occurrences(win, p.enabled)) {
		for _, d := range o.Days() {
			k := d.Format("2006-01-02")
			out[k] = append(out[k], o)
		}
	}
	return out
}

func (p *calendarPane) sidebarBody(w, h int) string {
	var b strings.Builder
	for i, c := range p.cals {
		check := IconCheckOff
		if p.enabled[c.ID] {
			check = IconCheckOn
		}
		dot := colorDot(c.Color)
		label := fmt.Sprintf("%s %s %s", check, dot, c.Name)
		if p.multiSource {
			label = fmt.Sprintf("%s %s %s %s", check, sourceGlyph(p.srcType[c.Source]), dot, c.Name)
		}
		sel := p.focus == focusLeft && i == p.sidebarIdx
		b.WriteString(p.theme.SelectStyle(sel, p.focus == focusLeft).Render(PadRight(Truncate(label, w), w)))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(p.miniMonth(w))
	return b.String()
}

func (p *calendarPane) miniMonth(w int) string {
	var b strings.Builder
	b.WriteString(p.theme.DayHeader.Render(p.anchor.Format("January 2006")) + "\n")
	b.WriteString(p.theme.Label.Render("Mo Tu We Th Fr Sa Su") + "\n")

	today := dayStart(time.Now())
	curDay := time.Time{}
	if o, ok := p.selectedOcc(); ok {
		curDay = o.Day()
	}
	hasEvents := p.daysWithEvents(p.anchor)

	first := startOfMonth(p.anchor)
	// Monday-based offset
	offset := (int(first.Weekday()) + 6) % 7
	day := first.AddDate(0, 0, -offset)
	for week := 0; week < 6; week++ {
		var cells []string
		for d := 0; d < 7; d++ {
			cell := fmt.Sprintf("%2d", day.Day())
			switch {
			case day.Month() != p.anchor.Month():
				cell = p.theme.ItemDim.Render(cell)
			case sameDay(day, today):
				cell = p.theme.Today.Render(cell)
			case sameDay(day, curDay):
				cell = p.theme.DayHeader.Render(cell)
			case hasEvents[day.Format("2006-01-02")]:
				cell = lipgloss.NewStyle().Foreground(p.theme.Accent).Render(cell)
			default:
				cell = p.theme.Value.Render(cell)
			}
			cells = append(cells, cell)
			day = day.AddDate(0, 0, 1)
		}
		b.WriteString(strings.Join(cells, " ") + "\n")
		if day.Month() != p.anchor.Month() && day.After(endOfMonth(p.anchor)) {
			break
		}
	}
	return b.String()
}

func (p *calendarPane) daysWithEvents(anchor time.Time) map[string]bool {
	out := map[string]bool{}
	win := model.DateRange{From: startOfMonth(anchor).AddDate(0, 0, -7), To: endOfMonth(anchor).AddDate(0, 0, 7)}
	for _, o := range p.filterOccs(p.store.Occurrences(win, p.enabled)) {
		for _, d := range o.Days() {
			out[d.Format("2006-01-02")] = true
		}
	}
	return out
}

func (p *calendarPane) agendaBody(w, h int) string {
	var b strings.Builder
	if p.searching {
		p.search.Width = w
		b.WriteString(p.search.View())
		b.WriteByte('\n')
		h = max0(h - 1)
	}
	if len(p.rows) == 0 {
		msg := "no events in range"
		if p.query != "" {
			msg = "no matching events"
		}
		b.WriteString(p.theme.ItemDim.Render(msg))
		return b.String()
	}
	lines := make([]string, len(p.rows))
	for i, r := range p.rows {
		if r.header {
			lines[i] = p.theme.DayHeader.Render(Truncate(r.day.Format("Mon 02 Jan"), w))
			continue
		}
		lines[i] = p.occRow(r.occ, w)
	}
	cursorRow := 0
	if len(p.selRows) > 0 {
		cursorRow = p.selRows[p.curIdx]
	}
	visible, top := scrollWindow(lines, cursorRow, max0(h))
	for i, line := range visible {
		rowIdx := top + i
		isCursor := len(p.selRows) > 0 && rowIdx == p.selRows[p.curIdx]
		if p.rows[rowIdx].header {
			b.WriteString(line)
		} else {
			b.WriteString(p.theme.SelectStyle(isCursor, p.focus == focusMiddle && !p.searching).Render(PadRight(line, w)))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (p *calendarPane) occRow(o model.Occurrence, w int) string {
	dot := colorDot(o.Color)
	when := o.Start.Format("15:04")
	if o.AllDay {
		when = "all-day"
	}
	repeat := ""
	if o.Event != nil && o.Event.Recurring() {
		repeat = " " + IconRepeat
	}
	line := fmt.Sprintf("%-7s %s %s%s", when, dot, oneLine(o.Summary), repeat)
	return Truncate(line, w)
}

// detailLines renders an icon-prefixed value that may span multiple lines (e.g.
// a multi-line address). It word-wraps to the pane width and indents every row
// after the first under the icon, so both the author's line breaks and long
// lines survive into the pane instead of being truncated.
func (p *calendarPane) detailLines(icon, value string, w int) string {
	iconW := lipgloss.Width(icon) + 1
	indent := strings.Repeat(" ", iconW)
	var b strings.Builder
	for i, line := range wrapLines(value, w-iconW) {
		prefix := icon + " "
		if i > 0 {
			prefix = indent
		}
		b.WriteString(p.theme.Value.Render(prefix+line) + "\n")
	}
	return b.String()
}

func (p *calendarPane) detailBody(w int) string {
	o, ok := p.selectedOcc()
	if !ok {
		return p.theme.ItemDim.Render("no selection")
	}
	var b strings.Builder
	b.WriteString(p.theme.Title.Render(Truncate(oneLine(o.Summary), w)) + "\n\n")

	when := o.Start.Format("Mon, Jan 2 2006")
	if o.AllDay {
		when += "  (all-day)"
	} else {
		when += fmt.Sprintf("  %s – %s", o.Start.Format("15:04"), o.End.Format("15:04"))
	}
	b.WriteString(p.theme.Value.Render(IconClock+" "+Truncate(when, w-2)) + "\n")

	if col, ok := p.colByID[o.CollectionID]; ok {
		b.WriteString(p.theme.Value.Render(colorDot(o.Color)+" "+Truncate(col.Name, w-2)) + "\n")
	}
	ev := o.Event
	if ev == nil {
		return b.String()
	}
	if ev.Location != "" {
		b.WriteString(p.detailLines(IconLocation, ev.Location, w))
	}
	if ev.Recurring() {
		b.WriteString(p.theme.Label.Render(IconRepeat+" "+Truncate(rruleSummary(ev.RRule), w-2)) + "\n")
	}
	if len(ev.Attendees) > 0 {
		b.WriteString(p.theme.Label.Render(fmt.Sprintf("%s %d attendee(s)", IconPerson, len(ev.Attendees))) + "\n")
	}
	if ev.Description != "" {
		b.WriteString("\n" + p.theme.Value.Render(wrap(ev.Description, w, 8)))
	}
	return b.String()
}

// ---- date helpers ----

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}
func endOfMonth(t time.Time) time.Time { return startOfMonth(t).AddDate(0, 1, 0).AddDate(0, 0, -1) }
func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func colorDot(c model.Color) string {
	if c.Hex() == "" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89")).Render(IconDot)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex())).Render(IconDot)
}

// rruleSummary gives a short human description of a recurrence rule.
func rruleSummary(rule string) string {
	rule = strings.ToUpper(rule)
	switch {
	case strings.Contains(rule, "FREQ=DAILY"):
		return "repeats daily"
	case strings.Contains(rule, "FREQ=WEEKLY"):
		return "repeats weekly"
	case strings.Contains(rule, "FREQ=MONTHLY"):
		return "repeats monthly"
	case strings.Contains(rule, "FREQ=YEARLY"):
		return "repeats yearly"
	default:
		return "recurring"
	}
}

// wrap word-wraps s to width w, preserving the author's line breaks, and clips
// to at most maxLines rows.
func wrap(s string, w, maxLines int) string {
	lines := wrapLines(s, w)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

// wrapLines word-wraps s to a display width of w, returning one entry per
// resulting row. Existing newlines in s are preserved as row breaks.
func wrapLines(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	return strings.Split(wordwrap.String(s, w), "\n")
}
