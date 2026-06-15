// Package model holds Yoro's pure domain types. It must not import I/O,
// bubbletea, or lipgloss so it can be shared by the store, the UI, and any
// future CalDAV/CardDAV backend without import cycles.
package model

import (
	"strings"
	"time"
)

// idSep separates a source id from a backend-native collection identifier (a
// filesystem path, a DAV href, …) inside a globally-unique Collection.ID. It is
// invisible and never appears in either part.
const idSep = "\x1f"

// NamespaceID builds a globally-unique collection ID from a source id and a
// backend-native identifier, so collections from different sources never collide.
func NamespaceID(sourceID, native string) string { return sourceID + idSep + native }

// NativeID recovers the backend-native identifier from a namespaced collection ID.
func NativeID(sourceID, id string) string { return strings.TrimPrefix(id, sourceID+idSep) }

// Kind distinguishes a calendar collection from an address book.
type Kind int

const (
	KindCalendar Kind = iota
	KindAddressBook
)

// Collection is a calendar or address book: one directory on disk, or one
// collection on a remote DAV server.
type Collection struct {
	ID     string // stable identifier, unique across sources (source-namespaced)
	Source string // id of the owning source (which backend it came from)
	Name   string // human display name (from the displayname file, may contain emoji)
	Color  Color  // normalized #RRGGBB, empty if none
	Kind   Kind
	Path   string // filesystem path for local backends; empty otherwise
}

// DateRange is a half-open time window [From, To).
type DateRange struct {
	From time.Time
	To   time.Time
}

// Contains reports whether t falls within the range.
func (r DateRange) Contains(t time.Time) bool {
	return !t.Before(r.From) && t.Before(r.To)
}

// Event is a base calendar event (a VEVENT). Recurring events keep their raw
// recurrence rules so the store can expand them into Occurrences on demand.
type Event struct {
	UID          string
	CollectionID string
	Summary      string
	Description  string
	Location     string
	Start        time.Time
	End          time.Time
	AllDay       bool

	// Recurrence rules, retained verbatim for windowed expansion.
	RRule   string
	RDates  []time.Time
	ExDates []time.Time

	Status    string
	Attendees []Attendee
	Alarms    []Alarm

	// Round-trip fidelity and write-back locator.
	Sequence int
	Rev      string
	ETag     string
	Raw      []byte
	Path     string // source file path (local) or object href (DAV), for updates
}

// Recurring reports whether the event repeats.
func (e Event) Recurring() bool { return e.RRule != "" || len(e.RDates) > 0 }

// Attendee is a participant of an event.
type Attendee struct {
	Name   string
	Email  string
	Status string // PARTSTAT, e.g. ACCEPTED / NEEDS-ACTION
	Role   string
}

// Alarm is a VALARM trigger.
type Alarm struct {
	Trigger     string // raw TRIGGER value, e.g. -PT2H
	Description string
}

// Todo is a VTODO task.
type Todo struct {
	UID          string
	CollectionID string
	Summary      string
	Status       string // NEEDS-ACTION / COMPLETED / ...
	Due          *time.Time
	Completed    *time.Time
	Raw          []byte
}

// Occurrence is a concrete, time-placed instance of an event. Non-recurring
// events produce exactly one; recurring events produce one per expansion.
type Occurrence struct {
	UID          string
	CollectionID string
	Color        Color
	Summary      string
	Start        time.Time
	End          time.Time
	AllDay       bool
	Event        *Event // back-reference to the base event for detail rendering
}

// Day returns the local calendar date (midnight) of the occurrence start.
func (o Occurrence) Day() time.Time {
	t := o.Start
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Contact is a vCard.
type Contact struct {
	UID          string
	CollectionID string
	FN           string // formatted name (preferred display)
	Name         StructuredName
	Emails       []TypedValue
	Phones       []TypedValue
	Org          string
	Title        string
	Birthday     *time.Time
	Note         string
	Photo        []byte // decoded PHOTO bytes when embedded; nil for URI-only

	Rev  string
	ETag string
	Raw  []byte
	Path string // source file path (local) or object href (DAV), for updates
}

// DisplayName returns the best available human label: the formatted name, then
// organization, then the first email or phone, then a placeholder.
func (c Contact) DisplayName() string {
	if c.FN != "" {
		return c.FN
	}
	if c.Org != "" {
		return c.Org
	}
	if len(c.Emails) > 0 {
		return c.Emails[0].Value
	}
	if len(c.Phones) > 0 {
		return c.Phones[0].Value
	}
	return "(no name)"
}

// HasName reports whether the contact has a real name (FN or structured N),
// as opposed to only an org/email/phone.
func (c Contact) HasName() bool {
	return c.FN != "" || c.Name.Family != "" || c.Name.Given != ""
}

// StructuredName is the components of a vCard N property.
type StructuredName struct {
	Family     string
	Given      string
	Additional string
	Prefix     string
	Suffix     string
}

// TypedValue is a value with its associated type labels (e.g. an email tagged
// HOME/WORK/pref).
type TypedValue struct {
	Value string
	Types []string
}
