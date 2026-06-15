package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
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
	focus     int // 0=sidebar, 1=agenda, 2=detail
	status    string
}

func newCalendarPane(theme Theme, keys KeyMap, st store.Store) *calendarPane {
	now := time.Now()
	return &calendarPane{
		theme:   theme,
		keys:    keys,
		store:   st,
		enabled: map[string]bool{},
		anchor:  startOfMonth(now),
		window:  model.DateRange{From: dayStart(now).AddDate(0, 0, -7), To: dayStart(now).AddDate(0, 0, 56)},
		focus:   1,
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
	occs := p.store.Occurrences(p.window, p.enabled)
	p.rows = p.rows[:0]
	p.selRows = p.selRows[:0]

	var lastDay time.Time
	add := func(day time.Time, occ model.Occurrence, header bool) {
		p.rows = append(p.rows, agendaRow{header: header, day: day, occ: occ})
		if !header {
			p.selRows = append(p.selRows, len(p.rows)-1)
		}
	}
	for _, o := range occs {
		d := o.Day()
		if !sameDay(d, lastDay) {
			add(d, model.Occurrence{}, true)
			lastDay = d
		}
		add(d, o, false)
	}
	p.curIdx = clamp(p.curIdx, 0, max0(len(p.selRows)-1))
}

func (p *calendarPane) Update(msg tea.Msg) (tea.Cmd, bool) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch {
	case key.Matches(km, p.keys.Left):
		if p.focus > 0 {
			p.focus--
		}
	case key.Matches(km, p.keys.Right):
		if p.focus < 2 {
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
	default:
		return nil, false
	}
	p.syncAnchor()
	return nil, true
}

func (p *calendarPane) moveDown(n int) {
	if p.focus == 0 {
		p.sidebarIdx = clamp(p.sidebarIdx+n, 0, max0(len(p.cals)-1))
		return
	}
	if p.curIdx >= len(p.selRows)-1 {
		p.extendForward()
	}
	p.cursorTo(p.curIdx + n)
}

func (p *calendarPane) moveUp(n int) {
	if p.focus == 0 {
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
	if p.focus != 0 || p.sidebarIdx >= len(p.cals) {
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
	if p.focus == 0 || len(p.selRows) == 0 {
		return
	}
	p.anchor = startOfMonth(p.rows[p.selRows[p.curIdx]].day)
}

func (p *calendarPane) extendForward() {
	p.window.To = p.window.To.AddDate(0, 0, 56)
	cur := p.curIdx
	p.rebuild()
	p.curIdx = cur
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

func (p *calendarPane) selectedOcc() (model.Occurrence, bool) {
	if p.curIdx < 0 || p.curIdx >= len(p.selRows) {
		return model.Occurrence{}, false
	}
	return p.rows[p.selRows[p.curIdx]].occ, true
}

// ---- rendering ----

func (p *calendarPane) View() string {
	w, h := p.width, p.height
	sideW := 26
	detailW := clamp(w*34/100, 26, 46)
	agendaW := w - sideW - detailW
	if agendaW < 18 {
		agendaW = 18
		detailW = max0(w - sideW - agendaW)
	}

	side := p.theme.Column("CALENDARS", p.sidebarBody(sideW-2, h-3), sideW, h, p.focus == 0)
	agenda := p.theme.Column("AGENDA", p.agendaBody(agendaW-2, h-3), agendaW, h, p.focus == 1)
	detail := p.theme.Column("EVENT", p.detailBody(detailW-2), detailW, h, p.focus == 2)
	return lipgloss.JoinHorizontal(lipgloss.Top, side, agenda, detail)
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
		sel := p.focus == 0 && i == p.sidebarIdx
		b.WriteString(p.theme.SelectStyle(sel, p.focus == 0).Render(PadRight(Truncate(label, w), w)))
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
	for _, o := range p.store.Occurrences(win, p.enabled) {
		out[o.Day().Format("2006-01-02")] = true
	}
	return out
}

func (p *calendarPane) agendaBody(w, h int) string {
	if len(p.rows) == 0 {
		return p.theme.ItemDim.Render("no events in range")
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
	var b strings.Builder
	for i, line := range visible {
		rowIdx := top + i
		isCursor := len(p.selRows) > 0 && rowIdx == p.selRows[p.curIdx]
		if p.rows[rowIdx].header {
			b.WriteString(line)
		} else {
			b.WriteString(p.theme.SelectStyle(isCursor, p.focus == 1).Render(PadRight(line, w)))
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
