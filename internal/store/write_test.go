package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zackb/yoro/internal/model"
)

// TestUpdateEventLocalInPlace confirms an update rewrites the SAME file (no
// duplicate) and is reflected on reload.
func TestUpdateEventLocalInPlace(t *testing.T) {
	cal, con := writeSource(t, t.TempDir(), "personal")
	st := New(LocalSource("local", "Local", cal, con))
	ctx := context.Background()
	if err := st.Load(ctx); err != nil {
		t.Fatal(err)
	}
	colID := st.Calendars()[0].ID

	window := model.DateRange{From: mustTime(t, "2026-06-01T00:00:00Z"), To: mustTime(t, "2026-07-01T00:00:00Z")}
	var ev model.Event
	for _, o := range st.Occurrences(window, nil) {
		if o.Summary == "Standup" {
			ev = *o.Event
		}
	}
	if ev.UID == "" || ev.Path == "" {
		t.Fatalf("seed event not found or missing locator: %+v", ev)
	}

	ev.Summary = "Renamed"
	if err := st.UpdateEvent(ctx, colID, ev); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}

	files, _ := os.ReadDir(filepath.Join(cal, "personal"))
	if len(files) != 1 {
		t.Fatalf("update should rewrite in place; got %d files", len(files))
	}
	if !containsSummary(st.Occurrences(window, nil), "Renamed") {
		t.Fatal("update not reflected after reload")
	}
}

// TestUpdateEventMissingPath rejects an update without a write-back locator.
func TestUpdateEventMissingPath(t *testing.T) {
	cal, con := writeSource(t, t.TempDir(), "personal")
	st := New(LocalSource("local", "Local", cal, con))
	ctx := context.Background()
	if err := st.Load(ctx); err != nil {
		t.Fatal(err)
	}
	colID := st.Calendars()[0].ID
	err := st.UpdateEvent(ctx, colID, model.Event{UID: "e1", Summary: "x"}) // no Path
	if err == nil {
		t.Fatal("expected error updating without Path")
	}
}

// TestCreateEventLocal writes a new event through the store into a local source
// and confirms it is readable back via Occurrences.
func TestCreateEventLocal(t *testing.T) {
	cal, con := writeSource(t, t.TempDir(), "personal")
	st := New(LocalSource("local", "Local", cal, con))
	ctx := context.Background()
	if err := st.Load(ctx); err != nil {
		t.Fatal(err)
	}
	colID := st.Calendars()[0].ID

	if err := st.CreateEvent(ctx, colID, model.Event{
		Summary: "Created",
		Start:   mustTime(t, "2026-06-20T15:00:00Z"),
		End:     mustTime(t, "2026-06-20T16:00:00Z"),
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	occs := st.Occurrences(model.DateRange{
		From: mustTime(t, "2026-06-01T00:00:00Z"),
		To:   mustTime(t, "2026-07-01T00:00:00Z"),
	}, nil)
	if !containsSummary(occs, "Created") {
		t.Fatalf("created event not found among %d occurrences", len(occs))
	}
}

// TestCreateContactLocal writes a new contact through the store and reads it back.
func TestCreateContactLocal(t *testing.T) {
	cal, con := writeSource(t, t.TempDir(), "personal")
	st := New(LocalSource("local", "Local", cal, con))
	ctx := context.Background()
	if err := st.Load(ctx); err != nil {
		t.Fatal(err)
	}
	colID := st.AddressBooks()[0].ID

	if err := st.CreateContact(ctx, colID, model.Contact{
		FN:     "Grace Hopper",
		Emails: []model.TypedValue{{Value: "grace@example.com"}},
	}); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	var found bool
	for _, c := range st.Contacts(colID) {
		if c.FN == "Grace Hopper" {
			found = true
		}
	}
	if !found {
		t.Fatal("created contact not found")
	}
}

// TestDeleteEventLocal creates an event, deletes it by its path, and confirms
// the file is gone and the occurrence no longer appears after reload.
func TestDeleteEventLocal(t *testing.T) {
	cal, con := writeSource(t, t.TempDir(), "personal")
	st := New(LocalSource("local", "Local", cal, con))
	ctx := context.Background()
	if err := st.Load(ctx); err != nil {
		t.Fatal(err)
	}
	colID := st.Calendars()[0].ID
	window := model.DateRange{From: mustTime(t, "2026-06-01T00:00:00Z"), To: mustTime(t, "2026-07-01T00:00:00Z")}

	if err := st.CreateEvent(ctx, colID, model.Event{
		Summary: "Doomed",
		Start:   mustTime(t, "2026-06-20T15:00:00Z"),
		End:     mustTime(t, "2026-06-20T16:00:00Z"),
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	var path string
	for _, o := range st.Occurrences(window, nil) {
		if o.Summary == "Doomed" {
			path = o.Event.Path
		}
	}
	if path == "" {
		t.Fatal("created event missing path")
	}

	if err := st.DeleteEvent(ctx, colID, path); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be removed; stat err = %v", err)
	}
	if containsSummary(st.Occurrences(window, nil), "Doomed") {
		t.Fatal("deleted event still present after reload")
	}
}

// TestDeleteContactLocal creates a contact, deletes it by path, and confirms it
// is gone after reload.
func TestDeleteContactLocal(t *testing.T) {
	cal, con := writeSource(t, t.TempDir(), "personal")
	st := New(LocalSource("local", "Local", cal, con))
	ctx := context.Background()
	if err := st.Load(ctx); err != nil {
		t.Fatal(err)
	}
	colID := st.AddressBooks()[0].ID

	if err := st.CreateContact(ctx, colID, model.Contact{FN: "Katherine Johnson"}); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	var path string
	for _, c := range st.Contacts(colID) {
		if c.FN == "Katherine Johnson" {
			path = c.Path
		}
	}
	if path == "" {
		t.Fatal("created contact missing path")
	}

	if err := st.DeleteContact(ctx, colID, path); err != nil {
		t.Fatalf("DeleteContact: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be removed; stat err = %v", err)
	}
	for _, c := range st.Contacts(colID) {
		if c.FN == "Katherine Johnson" {
			t.Fatal("deleted contact still present after reload")
		}
	}
}

// TestDeleteMissingPath rejects a delete without a write-back locator.
func TestDeleteMissingPath(t *testing.T) {
	cal, con := writeSource(t, t.TempDir(), "personal")
	st := New(LocalSource("local", "Local", cal, con))
	ctx := context.Background()
	if err := st.Load(ctx); err != nil {
		t.Fatal(err)
	}
	colID := st.Calendars()[0].ID
	if err := st.DeleteEvent(ctx, colID, ""); err == nil {
		t.Fatal("expected error deleting without Path")
	}
}

// TestCreateReadOnlySource confirms a create against a source whose backend does
// not implement WriteBackend returns ErrReadOnly.
func TestCreateReadOnlySource(t *testing.T) {
	col := model.Collection{ID: "ro\x1fcal", Source: "ro", Kind: model.KindCalendar, Name: "Cal"}
	st := New(Source{
		SourceInfo: SourceInfo{ID: "ro", Name: "ro", Type: "ro"},
		Backend:    readOnlyBackend{col: col},
	})
	ctx := context.Background()
	if err := st.Load(ctx); err != nil {
		t.Fatal(err)
	}
	err := st.CreateEvent(ctx, col.ID, model.Event{Summary: "X", Start: time.Now(), End: time.Now()})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("want ErrReadOnly, got %v", err)
	}
}

// TestSafeName accepts plain UIDs and rejects any that aren't a single path
// element, so a crafted UID can't escape the collection directory.
func TestSafeName(t *testing.T) {
	if got, err := safeName("event-123", ".ics"); err != nil || got != "event-123.ics" {
		t.Fatalf("safeName(plain) = %q, %v; want event-123.ics, nil", got, err)
	}
	for _, bad := range []string{"", ".", "..", "../escape", "a/b", `a\b`, "/abs"} {
		if _, err := safeName(bad, ".ics"); err == nil {
			t.Errorf("safeName(%q) = nil error; want rejection", bad)
		}
	}
}

// TestCreateEventRejectsUnsafeUID confirms a create carrying a traversing UID is
// refused rather than writing a file outside the collection directory.
func TestCreateEventRejectsUnsafeUID(t *testing.T) {
	cal, con := writeSource(t, t.TempDir(), "personal")
	st := New(LocalSource("local", "Local", cal, con))
	ctx := context.Background()
	if err := st.Load(ctx); err != nil {
		t.Fatal(err)
	}
	colID := st.Calendars()[0].ID
	err := st.CreateEvent(ctx, colID, model.Event{
		UID:     "../../escape",
		Summary: "Evil",
		Start:   mustTime(t, "2026-06-20T15:00:00Z"),
		End:     mustTime(t, "2026-06-20T16:00:00Z"),
	})
	if err == nil {
		t.Fatal("expected CreateEvent to reject a traversing UID")
	}
	if _, statErr := os.Stat(filepath.Join(cal, "escape.ics")); !os.IsNotExist(statErr) {
		t.Fatalf("a file escaped the collection dir: %v", statErr)
	}
}

func containsSummary(occs []model.Occurrence, summary string) bool {
	for _, o := range occs {
		if o.Summary == summary {
			return true
		}
	}
	return false
}

// readOnlyBackend implements Backend but not WriteBackend.
type readOnlyBackend struct{ col model.Collection }

func (b readOnlyBackend) Collections(context.Context) ([]model.Collection, error) {
	return []model.Collection{b.col}, nil
}
func (b readOnlyBackend) Events(context.Context, string) ([]model.Event, error)     { return nil, nil }
func (b readOnlyBackend) Todos(context.Context, string) ([]model.Todo, error)       { return nil, nil }
func (b readOnlyBackend) Contacts(context.Context, string) ([]model.Contact, error) { return nil, nil }
