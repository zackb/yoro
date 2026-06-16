package ical

import (
	"strings"
	"testing"
	"time"

	"github.com/zackb/yoro/internal/model"
)

// TestUpdateEventPreservesFidelity confirms editing one event in a multi-event
// file changes only that event's modeled fields (bumping SEQUENCE) while leaving
// its unmodeled properties and any sibling component untouched.
func TestUpdateEventPreservesFidelity(t *testing.T) {
	const raw = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//other//app//EN
BEGIN:VEVENT
UID:keep-1
DTSTAMP:20260601T000000Z
DTSTART:20260615T090000Z
DTEND:20260615T093000Z
SUMMARY:Keep me
END:VEVENT
BEGIN:VEVENT
UID:edit-1
DTSTAMP:20260601T000000Z
DTSTART:20260616T120000Z
DTEND:20260616T130000Z
SUMMARY:Old title
DESCRIPTION:Important notes
SEQUENCE:2
X-CUSTOM:hello
END:VEVENT
END:VCALENDAR
`
	newStart := time.Date(2026, 6, 16, 14, 0, 0, 0, time.UTC)
	cal, err := UpdateEvent([]byte(raw), model.Event{
		UID: "edit-1", Summary: "New title", Start: newStart, End: newStart.Add(time.Hour),
		Description: "Revised notes", URL: "https://example.com/e",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Marshal(cal)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "X-CUSTOM:hello") {
		t.Errorf("unmodeled X- property was dropped:\n%s", data)
	}

	f, err := Parse(data, "c")
	if err != nil {
		t.Fatal(err)
	}
	var edited, kept *model.Event
	for i := range f.Events {
		switch f.Events[i].UID {
		case "edit-1":
			edited = &f.Events[i]
		case "keep-1":
			kept = &f.Events[i]
		}
	}
	if edited == nil || kept == nil {
		t.Fatalf("want both events back, got %d", len(f.Events))
	}
	if edited.Summary != "New title" {
		t.Errorf("summary: %q", edited.Summary)
	}
	if !edited.Start.Equal(newStart) {
		t.Errorf("start: got %s want %s", edited.Start, newStart)
	}
	if edited.Sequence != 3 {
		t.Errorf("SEQUENCE not bumped: %d", edited.Sequence)
	}
	if edited.Description != "Revised notes" {
		t.Errorf("form-owned DESCRIPTION not updated: %q", edited.Description)
	}
	if edited.URL != "https://example.com/e" {
		t.Errorf("URL not written: %q", edited.URL)
	}
	if kept.Summary != "Keep me" {
		t.Errorf("sibling event clobbered: %q", kept.Summary)
	}
}

// TestUpdateEventClearsBlankFields confirms the form now owns Location/
// Description/URL: a blank value removes the property rather than preserving it.
func TestUpdateEventClearsBlankFields(t *testing.T) {
	const raw = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//other//app//EN
BEGIN:VEVENT
UID:edit-1
DTSTAMP:20260601T000000Z
DTSTART:20260616T120000Z
DTEND:20260616T130000Z
SUMMARY:Title
LOCATION:Old room
DESCRIPTION:Old notes
URL:https://old.example
END:VEVENT
END:VCALENDAR
`
	start := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	cal, err := UpdateEvent([]byte(raw), model.Event{
		UID: "edit-1", Summary: "Title", Start: start, End: start.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Marshal(cal)
	if err != nil {
		t.Fatal(err)
	}
	for _, prop := range []string{"LOCATION", "DESCRIPTION", "URL"} {
		if strings.Contains(string(data), prop+":") {
			t.Errorf("blank %s not cleared:\n%s", prop, data)
		}
	}
}

// TestUpdateEventMissingUID errors rather than corrupting the file.
func TestUpdateEventMissingUID(t *testing.T) {
	const raw = "BEGIN:VCALENDAR\nVERSION:2.0\nBEGIN:VEVENT\nUID:a\nDTSTAMP:20260601T000000Z\nDTSTART:20260615T090000Z\nEND:VEVENT\nEND:VCALENDAR\n"
	if _, err := UpdateEvent([]byte(raw), model.Event{UID: "nope", Summary: "x"}); err == nil {
		t.Fatal("expected error for missing UID")
	}
}

// TestBuildEventRoundTrip confirms a built event marshals to .ics bytes that
// Parse reads back faithfully, including text escaping and the timed instant.
func TestBuildEventRoundTrip(t *testing.T) {
	start := time.Date(2026, 6, 20, 15, 0, 0, 0, time.Local)
	e := model.Event{
		UID:      "e-1",
		Summary:  "Team sync, daily", // comma must round-trip via escaping
		Location: "Room 1\nFloor 2",  // newline must round-trip
		Start:    start,
		End:      start.Add(30 * time.Minute),
	}
	data, err := Marshal(BuildEvent(e))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(data, "col")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(f.Events))
	}
	got := f.Events[0]
	if got.UID != "e-1" || got.Summary != "Team sync, daily" {
		t.Errorf("uid/summary: %q %q", got.UID, got.Summary)
	}
	if got.Location != "Room 1\nFloor 2" {
		t.Errorf("location escaping not round-tripped: %q", got.Location)
	}
	if !got.Start.Equal(start) { // Equal compares instants across zones
		t.Errorf("start instant: got %s want %s", got.Start, start)
	}
	if got.End.Sub(got.Start) != 30*time.Minute {
		t.Errorf("duration: %s", got.End.Sub(got.Start))
	}
	if got.AllDay {
		t.Error("should not be all-day")
	}
}

// TestBuildAllDayEventRoundTrip confirms a blank-time event encodes as a DATE.
func TestBuildAllDayEventRoundTrip(t *testing.T) {
	day := time.Date(2026, 6, 20, 0, 0, 0, 0, time.Local)
	e := model.Event{UID: "e-2", Summary: "Holiday", Start: day, End: day.AddDate(0, 0, 1), AllDay: true}
	data, err := Marshal(BuildEvent(e))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(data, "col")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Events) != 1 || !f.Events[0].AllDay {
		t.Fatalf("want 1 all-day event, got %+v", f.Events)
	}
	if f.Events[0].Start.Format("2006-01-02") != "2026-06-20" {
		t.Errorf("start date: %s", f.Events[0].Start)
	}
}
