package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zackb/yoro/internal/model"
)

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
