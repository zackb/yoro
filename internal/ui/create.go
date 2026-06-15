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

// formField is one labeled text input in the create form.
type formField struct {
	key   string
	label string
	input textinput.Model
}

// createForm is the modal overlay for creating a new event or contact. It is a
// pure input widget: App reads its values on submit, persists via the store, and
// shows any error back in f.err. Single required field per domain (summary/name);
// the rest are optional.
type createForm struct {
	theme  Theme
	domain Mode
	target model.Collection
	source string // owning source display name, for provenance in the header
	fields []formField
	focus  int
	err    string

	editing     bool // edit an existing item vs create a new one
	origEvent   model.Event
	origContact model.Contact
}

func newEventForm(theme Theme, target model.Collection, source string) *createForm {
	now := time.Now()
	f := &createForm{theme: theme, domain: ModeCalendar, target: target, source: source}
	f.fields = []formField{
		field("summary", "Summary", ""),
		field("date", "Date", now.Format("2006-01-02")),
		field("time", "Time", now.Add(time.Hour).Truncate(time.Hour).Format("15:04")),
		field("duration", "Duration", "60"),
	}
	f.fields[0].input.Focus()
	return f
}

func newContactForm(theme Theme, target model.Collection, source string) *createForm {
	f := &createForm{theme: theme, domain: ModeContacts, target: target, source: source}
	f.fields = []formField{
		field("name", "Name", ""),
		field("email", "Email", ""),
		field("phone", "Phone", ""),
	}
	f.fields[0].input.Focus()
	return f
}

// newEditEventForm pre-fills the form from an existing event. Times are shown in
// the local zone; a blank time means all-day.
func newEditEventForm(theme Theme, target model.Collection, source string, e model.Event) *createForm {
	f := &createForm{theme: theme, domain: ModeCalendar, target: target, source: source, editing: true, origEvent: e}
	start := e.Start.Local()
	timeVal := start.Format("15:04")
	if e.AllDay {
		timeVal = ""
	}
	dur := int(e.End.Sub(e.Start).Minutes())
	if dur <= 0 {
		dur = 60
	}
	f.fields = []formField{
		field("summary", "Summary", e.Summary),
		field("date", "Date", start.Format("2006-01-02")),
		field("time", "Time", timeVal),
		field("duration", "Duration", strconv.Itoa(dur)),
	}
	f.fields[0].input.Focus()
	return f
}

// newEditContactForm pre-fills the form from an existing contact (name + first
// email/phone). Other emails/phones and unmodeled fields are preserved on save.
func newEditContactForm(theme Theme, target model.Collection, source string, c model.Contact) *createForm {
	f := &createForm{theme: theme, domain: ModeContacts, target: target, source: source, editing: true, origContact: c}
	email, phone := "", ""
	if len(c.Emails) > 0 {
		email = c.Emails[0].Value
	}
	if len(c.Phones) > 0 {
		phone = c.Phones[0].Value
	}
	f.fields = []formField{
		field("name", "Name", c.FN),
		field("email", "Email", email),
		field("phone", "Phone", phone),
	}
	f.fields[0].input.Focus()
	return f
}

func field(key, label, val string) formField {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Width = 28
	ti.SetValue(val)
	ti.CursorEnd()
	return formField{key: key, label: label, input: ti}
}

// update processes one message, reporting whether the form was submitted (enter)
// or cancelled (esc); otherwise it forwards editing/navigation to the fields.
func (f *createForm) update(msg tea.Msg) (submitted, cancelled bool, cmd tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		f.fields[f.focus].input, cmd = f.fields[f.focus].input.Update(msg)
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
	}
	f.fields[f.focus].input, cmd = f.fields[f.focus].input.Update(msg)
	return false, false, cmd
}

func (f *createForm) focusBy(d int) {
	f.fields[f.focus].input.Blur()
	f.focus = (f.focus + d + len(f.fields)) % len(f.fields)
	f.fields[f.focus].input.Focus()
}

func (f *createForm) get(key string) string {
	for i := range f.fields {
		if f.fields[i].key == key {
			return strings.TrimSpace(f.fields[i].input.Value())
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
	if f.editing {
		ev.UID, ev.Path, ev.Raw = f.origEvent.UID, f.origEvent.Path, f.origEvent.Raw
	}
	return ev, nil
}

// buildContact parses the contact fields into a new Contact.
func (f *createForm) buildContact() (model.Contact, error) {
	name := f.get("name")
	if name == "" {
		return model.Contact{}, errors.New("name is required")
	}
	c := model.Contact{FN: name}
	if e := f.get("email"); e != "" {
		c.Emails = []model.TypedValue{{Value: e}}
	}
	if p := f.get("phone"); p != "" {
		c.Phones = []model.TypedValue{{Value: p}}
	}
	if f.editing {
		c.UID, c.Path, c.Raw = f.origContact.UID, f.origContact.Path, f.origContact.Raw
	}
	return c, nil
}

// view renders the form centered in the given terminal size.
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

	var b strings.Builder
	b.WriteString(f.theme.Title.Render(header) + "\n\n")
	for i := range f.fields {
		marker := "  "
		if i == f.focus {
			marker = f.theme.StatusKey.Render("▸ ")
		}
		b.WriteString(marker + f.theme.Label.Render(PadRight(f.fields[i].label, 9)) + " " + f.fields[i].input.View() + "\n")
	}
	if f.domain == ModeCalendar {
		b.WriteString("\n" + f.theme.ItemDim.Render("blank time = all-day") + "\n")
	}
	if f.err != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render(f.err) + "\n")
	}
	b.WriteString("\n" + f.theme.Help.Render("enter save · tab next · esc cancel"))

	content := b.String()
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		f.theme.Column("", content, 50, lipgloss.Height(content)+2, true))
}
