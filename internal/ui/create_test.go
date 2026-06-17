package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zackb/yoro/internal/model"
)

func typeRunes(f *createForm, s string) {
	for _, r := range s {
		f.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func press(f *createForm, t tea.KeyType) { f.update(tea.KeyMsg{Type: t}) }

// TestContactFormBuildsAllFields drives the form like a user: it fills the
// structured name, adds extra typed rows, cycles a type, fills an address, and
// confirms buildContact captures everything. It also renders at a short height
// to exercise scrolling without panicking.
func TestContactFormBuildsAllFields(t *testing.T) {
	book := model.Collection{Name: "Personal", Kind: model.KindAddressBook}
	f := newContactForm(DefaultTheme(), book, "local")

	// Render small to exercise the scroll window.
	if f.view(60, 14) == "" {
		t.Fatal("empty form view")
	}

	// Focus is on the first field (Prefix). Type the name components.
	typeRunes(f, "Dr.")
	press(f, tea.KeyTab) // First
	typeRunes(f, "Ada")
	press(f, tea.KeyTab) // Middle
	press(f, tea.KeyTab) // Last
	typeRunes(f, "Lovelace")

	// Jump to the first email row and fill it, then add a second.
	f.focus = indexOfGroup(f, "email")
	f.syncFocus()
	typeRunes(f, "ada@home.example")
	f.update(tea.KeyMsg{Type: tea.KeyCtrlT}) // cycle type home->work
	f.update(tea.KeyMsg{Type: tea.KeyCtrlN}) // add a second email row (now focused)
	typeRunes(f, "ada@work.example")

	// Fill the address (focus its first sub-input: Street).
	f.focus = indexOfGroup(f, "address")
	f.syncFocus()
	typeRunes(f, "1 Engine Way")
	press(f, tea.KeyTab) // City
	typeRunes(f, "London")

	c, err := f.buildContact()
	if err != nil {
		t.Fatalf("buildContact: %v", err)
	}
	if c.Name.Given != "Ada" || c.Name.Family != "Lovelace" || c.Name.Prefix != "Dr." {
		t.Errorf("structured name: %+v", c.Name)
	}
	if c.FN != "Dr. Ada Lovelace" {
		t.Errorf("derived FN: %q", c.FN)
	}
	if len(c.Emails) != 2 {
		t.Fatalf("emails: %+v", c.Emails)
	}
	if c.Emails[0].Value != "ada@home.example" || len(c.Emails[0].Types) == 0 || c.Emails[0].Types[0] != "work" {
		t.Errorf("first email/type: %+v", c.Emails[0])
	}
	if c.Emails[1].Value != "ada@work.example" {
		t.Errorf("second email: %+v", c.Emails[1])
	}
	if len(c.Addresses) != 1 || c.Addresses[0].Street != "1 Engine Way" || c.Addresses[0].Locality != "London" {
		t.Errorf("address: %+v", c.Addresses)
	}
}

// TestContactFormRequiresName rejects an empty name.
func TestContactFormRequiresName(t *testing.T) {
	f := newContactForm(DefaultTheme(), model.Collection{Name: "B"}, "")
	if _, err := f.buildContact(); err == nil {
		t.Fatal("expected error for missing name")
	}
}

// TestContactFormDeleteLastRowClears confirms removing the only row of a group
// clears it instead of dropping the group (so it stays addable).
func TestContactFormDeleteLastRowClears(t *testing.T) {
	f := newContactForm(DefaultTheme(), model.Collection{Name: "B"}, "")
	before := f.groupCount("email")
	f.focus = indexOfGroup(f, "email")
	f.syncFocus()
	typeRunes(f, "x@y.z")
	f.update(tea.KeyMsg{Type: tea.KeyCtrlD}) // delete -> should clear, not remove
	if got := f.groupCount("email"); got != before {
		t.Errorf("group count changed: before %d after %d", before, got)
	}
	for i := range f.fields {
		if f.fields[i].group == "email" && f.fields[i].input.Value() != "" {
			t.Errorf("email row not cleared: %q", f.fields[i].input.Value())
		}
	}
}

// TestEventFormBuildsNewFields confirms the calendar form captures location,
// description and URL.
func TestEventFormBuildsNewFields(t *testing.T) {
	f := newEventForm(DefaultTheme(), model.Collection{Name: "Cal"}, "")
	setText(f, "summary", "Standup")
	setText(f, "location", "Room 5")
	setText(f, "description", "daily sync")
	setText(f, "url", "https://meet.example")
	ev, err := f.buildEvent()
	if err != nil {
		t.Fatalf("buildEvent: %v", err)
	}
	if ev.Location != "Room 5" || ev.Description != "daily sync" || ev.URL != "https://meet.example" {
		t.Errorf("event fields: loc=%q desc=%q url=%q", ev.Location, ev.Description, ev.URL)
	}
}

// TestComposeRRule covers frequency, interval and until serialization for both
// timed and all-day events.
func TestComposeRRule(t *testing.T) {
	cases := []struct {
		freq, interval, until string
		allDay                bool
		want                  string
	}{
		{"None", "1", "", false, ""},
		{"Daily", "1", "", false, "FREQ=DAILY"},
		{"Weekly", "2", "", false, "FREQ=WEEKLY;INTERVAL=2"},
		{"Monthly", "", "", false, "FREQ=MONTHLY"},
		{"Daily", "1", "2026-12-31", false, "FREQ=DAILY;UNTIL=20261231T235959Z"},
		{"Yearly", "1", "2026-12-31", true, "FREQ=YEARLY;UNTIL=20261231"},
	}
	for _, c := range cases {
		if got := composeRRule(c.freq, c.interval, c.until, c.allDay); got != c.want {
			t.Errorf("composeRRule(%q,%q,%q,allDay=%v) = %q, want %q", c.freq, c.interval, c.until, c.allDay, got, c.want)
		}
	}
}

// TestParseRRule covers the picker-modeled subset and the modeled flag that
// flags rules the picker can't represent.
func TestParseRRule(t *testing.T) {
	cases := []struct {
		rule                  string
		freq, interval, until string
		modeled               bool
	}{
		{"", "None", "1", "", true},
		{"FREQ=DAILY", "Daily", "1", "", true},
		{"FREQ=WEEKLY;INTERVAL=2", "Weekly", "2", "", true},
		{"FREQ=MONTHLY;UNTIL=20261231T235959Z", "Monthly", "1", "2026-12-31", true},
		{"FREQ=WEEKLY;BYDAY=MO,WE,FR", "Weekly", "1", "", false},
		{"FREQ=DAILY;COUNT=10", "Daily", "1", "", false},
		{"FREQ=HOURLY", "None", "1", "", false},
	}
	for _, c := range cases {
		freq, interval, until, modeled := parseRRule(c.rule)
		if freq != c.freq || interval != c.interval || until != c.until || modeled != c.modeled {
			t.Errorf("parseRRule(%q) = (%q,%q,%q,%v), want (%q,%q,%q,%v)",
				c.rule, freq, interval, until, modeled, c.freq, c.interval, c.until, c.modeled)
		}
	}
}

// TestEventFormBuildsRecurrence drives the picker and confirms buildEvent emits
// the composed rule.
func TestEventFormBuildsRecurrence(t *testing.T) {
	f := newEventForm(DefaultTheme(), model.Collection{Name: "Cal"}, "")
	setText(f, "summary", "Standup")
	setChoice(f, "repeat", "Weekly")
	setText(f, "interval", "2")
	ev, err := f.buildEvent()
	if err != nil {
		t.Fatalf("buildEvent: %v", err)
	}
	if ev.RRule != "FREQ=WEEKLY;INTERVAL=2" {
		t.Errorf("rrule: %q", ev.RRule)
	}
}

// TestEventFormPreservesUnmodeledRule confirms editing an event with a rule the
// picker can't represent keeps it verbatim when the cadence isn't touched, but
// regenerates from the picker once the user changes it.
func TestEventFormPreservesUnmodeledRule(t *testing.T) {
	start := time.Date(2026, 6, 15, 9, 0, 0, 0, time.Local)
	orig := model.Event{
		UID: "e-1", Summary: "Standup", Start: start, End: start.Add(time.Hour),
		RRule: "FREQ=WEEKLY;BYDAY=MO,WE,FR",
	}

	// Untouched: the BYDAY rule survives verbatim.
	f := newEditEventForm(DefaultTheme(), model.Collection{Name: "Cal"}, "", orig)
	ev, err := f.buildEvent()
	if err != nil {
		t.Fatalf("buildEvent: %v", err)
	}
	if ev.RRule != "FREQ=WEEKLY;BYDAY=MO,WE,FR" {
		t.Errorf("unmodeled rule not preserved: %q", ev.RRule)
	}

	// Changed cadence: regenerate from the picker (BYDAY intentionally dropped).
	f = newEditEventForm(DefaultTheme(), model.Collection{Name: "Cal"}, "", orig)
	setChoice(f, "repeat", "Daily")
	ev, err = f.buildEvent()
	if err != nil {
		t.Fatalf("buildEvent: %v", err)
	}
	if ev.RRule != "FREQ=DAILY" {
		t.Errorf("changed cadence not regenerated: %q", ev.RRule)
	}
}

// setChoice selects the given option on a kindChoice field by key.
func setChoice(f *createForm, key, opt string) {
	for i := range f.fields {
		fld := &f.fields[i]
		if fld.kind == kindChoice && fld.key == key {
			fld.typeIdx = indexOf(fld.types, opt)
			return
		}
	}
}

// indexOfGroup returns the focus index of the first input of the given group.
func indexOfGroup(f *createForm, group string) int {
	for i, r := range f.refs() {
		if f.fields[r.fi].group == group {
			return i
		}
	}
	return 0
}

// setText sets the value of a plain text field by key.
func setText(f *createForm, key, val string) {
	for i := range f.fields {
		if f.fields[i].kind == kindText && f.fields[i].key == key {
			f.fields[i].input.SetValue(val)
			return
		}
	}
}
