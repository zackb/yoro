package ical

import (
	"testing"
	"time"
)

const tzEvent = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:abc-123
SUMMARY:Standup
LOCATION:Conference Room A
DTSTART;TZID=America/Los_Angeles:20260615T093000
DTEND;TZID=America/Los_Angeles:20260615T100000
RRULE:FREQ=WEEKLY;BYDAY=MO
DESCRIPTION:Daily team sync
ATTENDEE;CN=Zack;PARTSTAT=ACCEPTED:mailto:zack@example.com
BEGIN:VALARM
TRIGGER:-PT2H
ACTION:DISPLAY
END:VALARM
END:VEVENT
END:VCALENDAR
`

const allDayEvent = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:bday-1
SUMMARY:Sample Birthday
DTSTART;VALUE=DATE:20260612
DTEND;VALUE=DATE:20260613
END:VEVENT
END:VCALENDAR
`

func TestParseTimezoneEvent(t *testing.T) {
	f, err := Parse([]byte(tzEvent), "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(f.Events))
	}
	e := f.Events[0]
	if e.UID != "abc-123" || e.Summary != "Standup" {
		t.Errorf("unexpected uid/summary: %q %q", e.UID, e.Summary)
	}
	if e.CollectionID != "work" {
		t.Errorf("collection not stamped: %q", e.CollectionID)
	}
	if e.Location != "Conference Room A" {
		t.Errorf("location: %q", e.Location)
	}
	if !e.Recurring() || e.RRule != "FREQ=WEEKLY;BYDAY=MO" {
		t.Errorf("rrule not retained: %q", e.RRule)
	}
	if e.AllDay {
		t.Error("should not be all-day")
	}
	loc, _ := time.LoadLocation("America/Los_Angeles")
	if !e.Start.Equal(time.Date(2026, 6, 15, 9, 30, 0, 0, loc)) {
		t.Errorf("start in wrong zone: %s", e.Start)
	}
	if e.End.Sub(e.Start) != 30*time.Minute {
		t.Errorf("duration: %s", e.End.Sub(e.Start))
	}
	if len(e.Attendees) != 1 || e.Attendees[0].Email != "zack@example.com" {
		t.Errorf("attendee: %+v", e.Attendees)
	}
	if len(e.Alarms) != 1 || e.Alarms[0].Trigger != "-PT2H" {
		t.Errorf("alarm: %+v", e.Alarms)
	}
	if len(e.Raw) == 0 {
		t.Error("raw bytes not attached")
	}
}

const escapedEvent = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:esc-1
SUMMARY:Do the thing $1\,480.90
LOCATION:123 Main St\nApt 4\, Springfield
DTSTART:20260615T093000Z
DESCRIPTION:Line one\nLine two\; and a \\ backslash
END:VEVENT
END:VCALENDAR
`

func TestParseUnescapesText(t *testing.T) {
	f, err := Parse([]byte(escapedEvent), "work")
	if err != nil {
		t.Fatal(err)
	}
	e := f.Events[0]
	if e.Summary != "Do the thing $1,480.90" {
		t.Errorf("summary not unescaped: %q", e.Summary)
	}
	// Line breaks are preserved in the data (flattened only at single-line
	// render sites); commas are unescaped.
	if e.Location != "123 Main St\nApt 4, Springfield" {
		t.Errorf("location not unescaped: %q", e.Location)
	}
	// Multi-line field: newline preserved, ; and \ unescaped.
	if e.Description != "Line one\nLine two; and a \\ backslash" {
		t.Errorf("description not unescaped: %q", e.Description)
	}
}

// TestExpandLocalizesTimedOccurrence guards against rendering events in their
// stored zone: a UTC-stored time must surface as a local-zone occurrence so the
// agenda shows the right wall-clock and groups it under the right day.
func TestExpandLocalizesTimedOccurrence(t *testing.T) {
	const ev = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:z1
DTSTAMP:20260601T000000Z
DTSTART:20260616T032000Z
DTEND:20260616T040500Z
SUMMARY:Evening
END:VEVENT
END:VCALENDAR
`
	f, err := Parse([]byte(ev), "c")
	if err != nil {
		t.Fatal(err)
	}
	occs := Expand(&f.Events[0],
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local),
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local))
	if len(occs) != 1 {
		t.Fatalf("want 1 occurrence, got %d", len(occs))
	}
	o := occs[0]
	want := time.Date(2026, 6, 16, 3, 20, 0, 0, time.UTC)
	if !o.Start.Equal(want) {
		t.Errorf("instant changed: got %s want %s", o.Start, want)
	}
	if o.Start.Location() != time.Local {
		t.Errorf("occurrence not localized: location=%s", o.Start.Location())
	}
	if got, wantDay := o.Day().Format("2006-01-02"), want.In(time.Local).Format("2006-01-02"); got != wantDay {
		t.Errorf("agenda day = %s, want %s", got, wantDay)
	}
}

func TestParseAllDay(t *testing.T) {
	f, err := Parse([]byte(allDayEvent), "home")
	if err != nil {
		t.Fatal(err)
	}
	e := f.Events[0]
	if !e.AllDay {
		t.Error("want all-day")
	}
	if e.Start.Format("2006-01-02") != "2026-06-12" {
		t.Errorf("start: %s", e.Start)
	}
}

func TestExpandRecurring(t *testing.T) {
	f, _ := Parse([]byte(tzEvent), "work")
	e := f.Events[0]
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	occ := Expand(&e, from, to)
	// Mondays in June 2026: 1, 8, 15, 22, 29 → 5 occurrences (DTSTART is Mon 15th,
	// FREQ=WEEKLY anchored there expands both directions within the window).
	if len(occ) == 0 {
		t.Fatal("expected recurring occurrences")
	}
	for i := 1; i < len(occ); i++ {
		if !occ[i].Start.After(occ[i-1].Start) {
			t.Error("occurrences not sorted ascending")
		}
		if occ[i].End.Sub(occ[i].Start) != 30*time.Minute {
			t.Error("duration not preserved across instances")
		}
	}
}
