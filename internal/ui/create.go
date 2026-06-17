package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zackb/yoro/internal/model"
)

// labelW is the column width reserved for field labels.
const labelW = 12

// errStyle renders form validation errors.
var errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))

// Preset TYPE labels cycled with ctrl+t. "other" maps to no explicit TYPE.
var (
	emailTypes    = []string{"home", "work", "other"}
	phoneTypes    = []string{"cell", "home", "work", "fax", "other"}
	addrTypes     = []string{"home", "work", "other"}
	addrSubLabels = []string{"Street", "City", "State", "Zip", "Country"}
)

// fieldKind distinguishes a plain single-value field from the multi-value,
// typed rows (email/phone) and the structured address row.
type fieldKind int

const (
	kindText   fieldKind = iota // one labeled input
	kindTyped                   // value input + cyclable TYPE (email/phone)
	kindAddr                    // structured address: several inputs + TYPE
	kindChoice                  // cyclable fixed option list (e.g. recurrence)
)

// freqOptions are the recurrence frequencies the structured picker offers.
// "None" yields a non-recurring event.
var freqOptions = []string{"None", "Daily", "Weekly", "Monthly", "Yearly"}

// formField is one logical row in the create form.
type formField struct {
	key   string
	label string
	kind  fieldKind
	group string // "", "email", "phone", "address" — multi-value grouping

	input textinput.Model   // value input (kindText, kindTyped)
	sub   []textinput.Model // address components (kindAddr): street/city/state/zip/country

	types   []string // TYPE options (kindTyped, kindAddr)
	typeIdx int      // selected type

	poBox    string // address components not shown in the form, preserved round-trip
	extended string
}

// focusRef points at one editable input: a field, plus a sub-index for the
// multi-line address row.
type focusRef struct{ fi, si int }

// createForm is the modal overlay for creating or editing an event or contact.
// It is a pure input widget: App reads its values on submit, persists via the
// store, and shows any error back in f.err.
type createForm struct {
	theme  Theme
	domain Mode
	target model.Collection
	source string // owning source display name, for provenance in the header
	fields []formField
	focus  int // index into refs()
	scroll int // first visible body line when the form overflows
	err    string

	editing     bool // edit an existing item vs create a new one
	origEvent   model.Event
	origContact model.Contact
}

func newEventForm(theme Theme, target model.Collection, source string) *createForm {
	now := time.Now()
	f := &createForm{theme: theme, domain: ModeCalendar, target: target, source: source}
	f.fields = eventFields(model.Event{
		Start: now.Add(time.Hour).Truncate(time.Hour),
		End:   now.Add(2 * time.Hour).Truncate(time.Hour),
	})
	f.syncFocus()
	return f
}

func newContactForm(theme Theme, target model.Collection, source string) *createForm {
	f := &createForm{theme: theme, domain: ModeContacts, target: target, source: source}
	f.fields = contactFields(model.Contact{})
	f.syncFocus()
	return f
}

// newEditEventForm pre-fills the form from an existing event. Times are shown in
// the local zone; a blank time means all-day.
func newEditEventForm(theme Theme, target model.Collection, source string, e model.Event) *createForm {
	f := &createForm{theme: theme, domain: ModeCalendar, target: target, source: source, editing: true, origEvent: e}
	f.fields = eventFields(e)
	f.syncFocus()
	return f
}

// newEditContactForm pre-fills the form from an existing contact. Unmodeled
// fields are preserved on save.
func newEditContactForm(theme Theme, target model.Collection, source string, c model.Contact) *createForm {
	f := &createForm{theme: theme, domain: ModeContacts, target: target, source: source, editing: true, origContact: c}
	f.fields = contactFields(c)
	f.syncFocus()
	return f
}

// eventFields builds the calendar form rows from e (zero value for create).
func eventFields(e model.Event) []formField {
	timeVal := e.Start.Local().Format("15:04")
	if e.AllDay {
		timeVal = ""
	}
	dur := int(e.End.Sub(e.Start).Minutes())
	if dur <= 0 {
		dur = 60
	}
	freq, interval, until, _ := parseRRule(e.RRule)
	return []formField{
		field("summary", "Summary", e.Summary),
		field("date", "Date", e.Start.Local().Format("2006-01-02")),
		field("time", "Time", timeVal),
		field("duration", "Duration", strconv.Itoa(dur)),
		field("location", "Location", e.Location),
		field("description", "Description", e.Description),
		field("url", "URL", e.URL),
		choiceRow("repeat", "Repeat", freqOptions, indexOf(freqOptions, freq)),
		field("interval", "Every", interval),
		field("until", "Until", until),
	}
}

// parseRRule extracts the picker-modeled parts (FREQ, INTERVAL, UNTIL) from a
// raw rule. freq is a freqOptions label ("None" when absent or a FREQ the picker
// can't represent); interval defaults to "1"; until is reformatted to
// YYYY-MM-DD. modeled is false when the rule carries any component the picker
// doesn't expose (BYDAY, COUNT, …) or an unrepresentable FREQ — the caller uses
// this to preserve such rules verbatim unless the user edits the cadence.
func parseRRule(rule string) (freq, interval, until string, modeled bool) {
	freq, interval, modeled = "None", "1", true
	if strings.TrimSpace(rule) == "" {
		return freq, interval, until, modeled
	}
	for _, part := range strings.Split(rule, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(k)) {
		case "FREQ":
			switch strings.ToUpper(strings.TrimSpace(v)) {
			case "DAILY":
				freq = "Daily"
			case "WEEKLY":
				freq = "Weekly"
			case "MONTHLY":
				freq = "Monthly"
			case "YEARLY":
				freq = "Yearly"
			default:
				freq, modeled = "None", false
			}
		case "INTERVAL":
			interval = strings.TrimSpace(v)
		case "UNTIL":
			if d, ok := parseUntil(v); ok {
				until = d
			} else {
				modeled = false
			}
		default:
			modeled = false
		}
	}
	return freq, interval, until, modeled
}

// parseUntil reformats an RRULE UNTIL value (DATE or UTC DATE-TIME) to the
// form's YYYY-MM-DD.
func parseUntil(v string) (string, bool) {
	v = strings.TrimSpace(v)
	for _, layout := range []string{"20060102T150405Z", "20060102T150405", "20060102"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

// composeRRule builds a raw RRULE from the picker fields. It returns "" for the
// "None" frequency. UNTIL is written to match the DTSTART value type: a UTC
// date-time for timed events, a bare DATE for all-day ones (RFC 5545).
func composeRRule(freq, interval, until string, allDay bool) string {
	f := strings.ToUpper(strings.TrimSpace(freq))
	if f == "" || f == "NONE" {
		return ""
	}
	rule := "FREQ=" + f
	if n, err := strconv.Atoi(strings.TrimSpace(interval)); err == nil && n > 1 {
		rule += ";INTERVAL=" + strconv.Itoa(n)
	}
	if d, err := time.Parse("2006-01-02", strings.TrimSpace(until)); err == nil && until != "" {
		if allDay {
			rule += ";UNTIL=" + d.Format("20060102")
		} else {
			// Inclusive end-of-day in UTC so the final day's occurrence isn't clipped.
			end := time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, time.UTC)
			rule += ";UNTIL=" + end.Format("20060102T150405Z")
		}
	}
	return rule
}

func indexOf(opts []string, v string) int {
	for i, o := range opts {
		if o == v {
			return i
		}
	}
	return 0
}

// contactFields builds the contact form rows from c (zero value for create).
// Every multi-value group keeps at least one (possibly empty) row so it stays
// addable; empty rows are dropped on save.
func contactFields(c model.Contact) []formField {
	fields := []formField{
		field("prefix", "Prefix", c.Name.Prefix),
		field("first", "First", c.Name.Given),
		field("middle", "Middle", c.Name.Additional),
		field("last", "Last", c.Name.Family),
		field("suffix", "Suffix", c.Name.Suffix),
		field("nickname", "Nickname", c.Nickname),
		field("org", "Organization", c.Org),
		field("title", "Title", c.Title),
		field("role", "Role", c.Role),
	}
	if len(c.Emails) == 0 {
		fields = append(fields, typedRow("email", "Email", emailTypes, nil, ""))
	}
	for _, e := range c.Emails {
		fields = append(fields, typedRow("email", "Email", emailTypes, e.Types, e.Value))
	}
	if len(c.Phones) == 0 {
		fields = append(fields, typedRow("phone", "Phone", phoneTypes, nil, ""))
	}
	for _, p := range c.Phones {
		fields = append(fields, typedRow("phone", "Phone", phoneTypes, p.Types, p.Value))
	}
	if len(c.Addresses) == 0 {
		fields = append(fields, addressRow(addrTypes, nil, model.Address{}))
	}
	for _, a := range c.Addresses {
		fields = append(fields, addressRow(addrTypes, a.Types, a))
	}
	return append(fields,
		field("url", "URL", c.URL),
		field("birthday", "Birthday", formatDateField(c.Birthday)),
		field("anniversary", "Anniversary", formatDateField(c.Anniversary)),
		field("note", "Note", c.Note),
	)
}

func newInput(val string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Width = 26
	ti.SetValue(val)
	ti.CursorEnd()
	return ti
}

func field(key, label, val string) formField {
	return formField{key: key, label: label, kind: kindText, input: newInput(val)}
}

// choiceRow builds a cyclable fixed-option field. The selected option is stored
// in typeIdx and cycled with ←/→ or ctrl+t; the input is an unused placeholder
// so the focus machinery treats it like any other row.
func choiceRow(key, label string, options []string, sel int) formField {
	if sel < 0 || sel >= len(options) {
		sel = 0
	}
	return formField{key: key, label: label, kind: kindChoice, input: newInput(""), types: options, typeIdx: sel}
}

func typedRow(group, label string, presets, existing []string, val string) formField {
	types, idx := pickType(presets, existing)
	return formField{key: group, label: label, group: group, kind: kindTyped, input: newInput(val), types: types, typeIdx: idx}
}

func addressRow(presets, existing []string, a model.Address) formField {
	types, idx := pickType(presets, existing)
	return formField{
		key: "address", label: "Address", group: "address", kind: kindAddr,
		sub:   []textinput.Model{newInput(a.Street), newInput(a.Locality), newInput(a.Region), newInput(a.PostalCode), newInput(a.Country)},
		types: types, typeIdx: idx, poBox: a.POBox, extended: a.Extended,
	}
}

// pickType returns the TYPE options and the selected index. An existing type not
// among the presets is preserved by prepending it.
func pickType(presets, existing []string) ([]string, int) {
	if len(existing) == 0 {
		return presets, 0
	}
	want := strings.ToLower(strings.TrimSpace(existing[0]))
	for i, p := range presets {
		if p == want {
			return presets, i
		}
	}
	return append([]string{want}, presets...), 0
}

func formatDateField(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// update processes one message, reporting whether the form was submitted (enter)
// or cancelled (esc); otherwise it forwards editing/navigation to the inputs.
func (f *createForm) update(msg tea.Msg) (submitted, cancelled bool, cmd tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		ip := f.inputPtr(f.cur())
		*ip, cmd = ip.Update(msg)
		return false, false, cmd
	}
	switch km.String() {
	case "esc":
		return false, true, nil
	case "enter":
		return true, false, nil
	case "tab", "down":
		f.focusBy(1)
		return false, false, nil
	case "shift+tab", "up":
		f.focusBy(-1)
		return false, false, nil
	case "left":
		if f.curIsChoice() {
			f.cycleChoice(-1)
			return false, false, nil
		}
	case "right":
		if f.curIsChoice() {
			f.cycleChoice(1)
			return false, false, nil
		}
	case "ctrl+t":
		f.cycleType()
		return false, false, nil
	case "ctrl+n":
		f.addRow()
		return false, false, nil
	case "ctrl+d":
		f.delRow()
		return false, false, nil
	}
	ip := f.inputPtr(f.cur())
	*ip, cmd = ip.Update(msg)
	return false, false, cmd
}

// refs lists every editable input in display order.
func (f *createForm) refs() []focusRef {
	var r []focusRef
	for i := range f.fields {
		if f.fields[i].kind == kindAddr {
			for s := range f.fields[i].sub {
				r = append(r, focusRef{i, s})
			}
		} else {
			r = append(r, focusRef{i, 0})
		}
	}
	return r
}

func (f *createForm) cur() focusRef {
	rs := f.refs()
	if len(rs) == 0 {
		return focusRef{}
	}
	if f.focus < 0 {
		f.focus = 0
	}
	if f.focus >= len(rs) {
		f.focus = len(rs) - 1
	}
	return rs[f.focus]
}

func (f *createForm) inputPtr(ref focusRef) *textinput.Model {
	fld := &f.fields[ref.fi]
	if fld.kind == kindAddr {
		return &fld.sub[ref.si]
	}
	return &fld.input
}

// syncFocus blurs every input and focuses the current one.
func (f *createForm) syncFocus() {
	for i := range f.fields {
		f.fields[i].input.Blur()
		for s := range f.fields[i].sub {
			f.fields[i].sub[s].Blur()
		}
	}
	f.inputPtr(f.cur()).Focus()
}

func (f *createForm) focusBy(d int) {
	n := len(f.refs())
	if n == 0 {
		return
	}
	f.focus = (f.focus + d + n) % n
	f.syncFocus()
}

func (f *createForm) focusToField(fi int) {
	for i, r := range f.refs() {
		if r.fi == fi {
			f.focus = i
			break
		}
	}
	f.syncFocus()
}

func (f *createForm) cycleType() {
	fld := &f.fields[f.cur().fi]
	if len(fld.types) > 0 {
		fld.typeIdx = (fld.typeIdx + 1) % len(fld.types)
	}
}

// cycleChoice steps the focused choice field by d (wrapping). No-op otherwise.
func (f *createForm) cycleChoice(d int) {
	fld := &f.fields[f.cur().fi]
	n := len(fld.types)
	if fld.kind != kindChoice || n == 0 {
		return
	}
	fld.typeIdx = (fld.typeIdx + d + n) % n
}

func (f *createForm) curIsChoice() bool {
	return f.fields[f.cur().fi].kind == kindChoice
}

func (f *createForm) groupCount(g string) int {
	n := 0
	for i := range f.fields {
		if f.fields[i].group == g {
			n++
		}
	}
	return n
}

// addRow appends a new empty row to the focused multi-value group, after its
// last existing row, and moves focus to it. No-op on plain fields.
func (f *createForm) addRow() {
	g := f.fields[f.cur().fi].group
	if g == "" {
		return
	}
	ins := 0
	for j := range f.fields {
		if f.fields[j].group == g {
			ins = j + 1
		}
	}
	var nf formField
	switch g {
	case "email":
		nf = typedRow("email", "Email", emailTypes, nil, "")
	case "phone":
		nf = typedRow("phone", "Phone", phoneTypes, nil, "")
	case "address":
		nf = addressRow(addrTypes, nil, model.Address{})
	}
	f.fields = append(f.fields, formField{})
	copy(f.fields[ins+1:], f.fields[ins:])
	f.fields[ins] = nf
	f.focusToField(ins)
}

// delRow removes the focused multi-value row, or clears it when it is the last
// row of its group so the group stays addable. No-op on plain fields.
func (f *createForm) delRow() {
	ref := f.cur()
	g := f.fields[ref.fi].group
	if g == "" {
		return
	}
	if f.groupCount(g) <= 1 {
		f.fields[ref.fi].clear()
		return
	}
	f.fields = append(f.fields[:ref.fi], f.fields[ref.fi+1:]...)
	f.syncFocus()
}

func (fld *formField) clear() {
	fld.input.SetValue("")
	for s := range fld.sub {
		fld.sub[s].SetValue("")
	}
	fld.typeIdx = 0
}

func (fld *formField) selectedTypes() []string {
	if len(fld.types) == 0 {
		return nil
	}
	t := fld.types[fld.typeIdx]
	if t == "" || t == "other" {
		return nil
	}
	return []string{t}
}

func (fld *formField) toAddress() model.Address {
	g := func(i int) string { return strings.TrimSpace(fld.sub[i].Value()) }
	return model.Address{
		Types:      fld.selectedTypes(),
		POBox:      fld.poBox,
		Extended:   fld.extended,
		Street:     g(0),
		Locality:   g(1),
		Region:     g(2),
		PostalCode: g(3),
		Country:    g(4),
	}
}

func (f *createForm) get(key string) string {
	for i := range f.fields {
		if f.fields[i].kind == kindText && f.fields[i].key == key {
			return strings.TrimSpace(f.fields[i].input.Value())
		}
	}
	return ""
}

// choice returns the selected option of the named kindChoice field, or "".
func (f *createForm) choice(key string) string {
	for i := range f.fields {
		fld := &f.fields[i]
		if fld.kind == kindChoice && fld.key == key && len(fld.types) > 0 {
			return fld.types[fld.typeIdx]
		}
	}
	return ""
}

// buildEvent parses the calendar fields into a new Event. A blank time yields an
// all-day event; otherwise End = Start + duration minutes (default 60).
func (f *createForm) buildEvent() (model.Event, error) {
	summary := f.get("summary")
	if summary == "" {
		return model.Event{}, errors.New("summary is required")
	}
	day, err := time.ParseInLocation("2006-01-02", f.get("date"), time.Local)
	if err != nil {
		return model.Event{}, errors.New("date must be YYYY-MM-DD")
	}
	var ev model.Event
	if f.get("time") == "" {
		ev = model.Event{Summary: summary, Start: day, End: day.AddDate(0, 0, 1), AllDay: true}
	} else {
		tod, err := time.ParseInLocation("15:04", f.get("time"), time.Local)
		if err != nil {
			return model.Event{}, errors.New("time must be HH:MM")
		}
		start := time.Date(day.Year(), day.Month(), day.Day(), tod.Hour(), tod.Minute(), 0, 0, time.Local)
		dur := 60
		if d := f.get("duration"); d != "" {
			n, err := strconv.Atoi(d)
			if err != nil || n <= 0 {
				return model.Event{}, errors.New("duration must be a positive number of minutes")
			}
			dur = n
		}
		ev = model.Event{Summary: summary, Start: start, End: start.Add(time.Duration(dur) * time.Minute)}
	}
	ev.Location = f.get("location")
	ev.Description = f.get("description")
	ev.URL = f.get("url")
	ev.RRule = f.recurrence(ev.AllDay)
	if f.editing {
		ev.UID, ev.Path, ev.Raw = f.origEvent.UID, f.origEvent.Path, f.origEvent.Raw
	}
	return ev, nil
}

// recurrence builds the event's RRULE from the picker. To avoid dropping rule
// components the picker can't model (BYDAY, COUNT, …), an edited event whose
// recurrence rows are left exactly as loaded keeps its original rule verbatim;
// any change to the cadence regenerates from the picker.
func (f *createForm) recurrence(allDay bool) string {
	rule := composeRRule(f.choice("repeat"), f.get("interval"), f.get("until"), allDay)
	if f.editing && f.origEvent.RRule != "" {
		freq, interval, until, modeled := parseRRule(f.origEvent.RRule)
		if !modeled && rule == composeRRule(freq, interval, until, allDay) {
			return f.origEvent.RRule
		}
	}
	return rule
}

// buildContact parses the contact fields into a new Contact. FN is derived from
// the structured name; empty multi-value rows are dropped.
func (f *createForm) buildContact() (model.Contact, error) {
	name := model.StructuredName{
		Prefix:     f.get("prefix"),
		Given:      f.get("first"),
		Additional: f.get("middle"),
		Family:     f.get("last"),
		Suffix:     f.get("suffix"),
	}
	if name.Given == "" && name.Family == "" {
		return model.Contact{}, errors.New("first or last name is required")
	}
	c := model.Contact{
		FN:       deriveFN(name),
		Name:     name,
		Nickname: f.get("nickname"),
		Org:      f.get("org"),
		Title:    f.get("title"),
		Role:     f.get("role"),
		URL:      f.get("url"),
		Note:     f.get("note"),
	}
	bday, err := parseFormDate(f.get("birthday"))
	if err != nil {
		return model.Contact{}, errors.New("birthday must be YYYY-MM-DD")
	}
	c.Birthday = bday
	anniv, err := parseFormDate(f.get("anniversary"))
	if err != nil {
		return model.Contact{}, errors.New("anniversary must be YYYY-MM-DD")
	}
	c.Anniversary = anniv

	for i := range f.fields {
		fld := &f.fields[i]
		switch fld.group {
		case "email":
			if v := strings.TrimSpace(fld.input.Value()); v != "" {
				c.Emails = append(c.Emails, model.TypedValue{Value: v, Types: fld.selectedTypes()})
			}
		case "phone":
			if v := strings.TrimSpace(fld.input.Value()); v != "" {
				c.Phones = append(c.Phones, model.TypedValue{Value: v, Types: fld.selectedTypes()})
			}
		case "address":
			if a := fld.toAddress(); !a.Empty() {
				c.Addresses = append(c.Addresses, a)
			}
		}
	}
	if f.editing {
		c.UID, c.Path, c.Raw = f.origContact.UID, f.origContact.Path, f.origContact.Raw
	}
	return c, nil
}

// deriveFN joins the non-empty structured-name components into a formatted name.
func deriveFN(n model.StructuredName) string {
	var parts []string
	for _, p := range []string{n.Prefix, n.Given, n.Additional, n.Family, n.Suffix} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

func parseFormDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// typeBadge renders the current TYPE of a typed/address row.
func (f *createForm) typeBadge(fld *formField) string {
	label := "none"
	if len(fld.types) > 0 {
		label = fld.types[fld.typeIdx]
	}
	return f.theme.ItemDim.Render("[" + label + "]")
}

// bodyLines renders the field rows and reports the line index of the focused one.
func (f *createForm) bodyLines() (lines []string, focusLine int) {
	cur := f.cur()
	for i := range f.fields {
		fld := &f.fields[i]
		if fld.kind == kindAddr {
			lines = append(lines, "  "+f.theme.Label.Render(PadRight(fld.label, labelW))+" "+f.typeBadge(fld))
			for s := range fld.sub {
				marker := "  "
				if cur.fi == i && cur.si == s {
					marker = f.theme.StatusKey.Render("▸ ")
					focusLine = len(lines)
				}
				lines = append(lines, marker+f.theme.Label.Render(PadRight("  "+addrSubLabels[s], labelW))+" "+fld.sub[s].View())
			}
			continue
		}
		marker := "  "
		if cur.fi == i {
			marker = f.theme.StatusKey.Render("▸ ")
			focusLine = len(lines)
		}
		valView := fld.input.View()
		if fld.kind == kindChoice {
			opt := ""
			if len(fld.types) > 0 {
				opt = fld.types[fld.typeIdx]
			}
			valView = f.theme.Value.Render("‹ " + opt + " ›")
		}
		line := marker + f.theme.Label.Render(PadRight(fld.label, labelW)) + " " + valView
		if fld.kind == kindTyped {
			line += "  " + f.typeBadge(fld)
		}
		lines = append(lines, line)
	}
	return lines, focusLine
}

// view renders the form centered in the given terminal size, scrolling the field
// region so the focused field stays visible when the form overflows.
func (f *createForm) view(width, height int) string {
	verb := "New"
	if f.editing {
		verb = "Edit"
	}
	title := verb + " event"
	if f.domain == ModeContacts {
		title = verb + " contact"
	}
	header := fmt.Sprintf("%s — %s", title, f.target.Name)
	if f.source != "" {
		header += " (" + f.source + ")"
	}

	lines, focusLine := f.bodyLines()
	maxRows := height - 9
	if maxRows < 5 {
		maxRows = 5
	}
	var top, bottom bool
	if len(lines) > maxRows {
		if focusLine < f.scroll {
			f.scroll = focusLine
		}
		if focusLine >= f.scroll+maxRows {
			f.scroll = focusLine - maxRows + 1
		}
		if f.scroll > len(lines)-maxRows {
			f.scroll = len(lines) - maxRows
		}
		if f.scroll < 0 {
			f.scroll = 0
		}
		top = f.scroll > 0
		bottom = f.scroll+maxRows < len(lines)
		lines = lines[f.scroll : f.scroll+maxRows]
	} else {
		f.scroll = 0
	}

	var b strings.Builder
	b.WriteString(f.theme.Title.Render(header) + "\n\n")
	if top {
		b.WriteString(f.theme.ItemDim.Render("  ↑ more") + "\n")
	}
	b.WriteString(strings.Join(lines, "\n") + "\n")
	if bottom {
		b.WriteString(f.theme.ItemDim.Render("  ↓ more") + "\n")
	}
	if f.domain == ModeCalendar {
		b.WriteString("\n" + f.theme.ItemDim.Render("blank time = all-day") + "\n")
	}
	if f.err != "" {
		b.WriteString("\n" + errStyle.Render(f.err) + "\n")
	}
	b.WriteString("\n" + f.theme.Help.Render(f.helpText()))

	content := b.String()
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		f.theme.Column("", content, 54, lipgloss.Height(content)+2, true))
}

func (f *createForm) helpText() string {
	if f.domain == ModeContacts {
		return "enter save · tab next · ^t type · ^n add · ^d remove · esc cancel"
	}
	return "enter save · tab next · ←/→ repeat · esc cancel"
}
