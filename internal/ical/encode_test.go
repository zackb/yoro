package ical

import (
	"testing"
	"time"

	"github.com/zackb/yoro/internal/model"
)

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
