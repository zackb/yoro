package ui

import (
	"testing"

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
