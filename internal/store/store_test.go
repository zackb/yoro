package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zackb/yoro/internal/model"
)

const sampleICS = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//yoro//test//EN
BEGIN:VEVENT
UID:e1
DTSTAMP:20260601T000000Z
DTSTART:20260615T090000Z
DTEND:20260615T100000Z
SUMMARY:Standup
END:VEVENT
END:VCALENDAR
`

const sampleVCF = `BEGIN:VCARD
VERSION:3.0
UID:u1
FN:Ada Lovelace
END:VCARD
`

// writeSource lays out a local vdir tree with one calendar and one address book
// under root and returns the two roots.
func writeSource(t *testing.T, root, colName string) (calRoot, conRoot string) {
	t.Helper()
	calRoot = filepath.Join(root, "calendars")
	conRoot = filepath.Join(root, "contacts")
	calDir := filepath.Join(calRoot, colName)
	conDir := filepath.Join(conRoot, colName)
	for _, d := range []string{calDir, conDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(calDir, "e1.ics"), []byte(sampleICS), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conDir, "u1.vcf"), []byte(sampleVCF), 0o644); err != nil {
		t.Fatal(err)
	}
	return calRoot, conRoot
}

// TestMultiSourceAggregation verifies two sources with same-named collections
// coexist with unique, source-namespaced IDs and correct provenance.
func TestMultiSourceAggregation(t *testing.T) {
	aCal, aCon := writeSource(t, t.TempDir(), "personal")
	bCal, bCon := writeSource(t, t.TempDir(), "personal") // deliberately same name

	st := New(
		LocalSource("alpha", "Alpha", aCal, aCon),
		LocalSource("beta", "Beta", bCal, bCon),
	)
	if err := st.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := len(st.Sources()); got != 2 {
		t.Fatalf("Sources() = %d, want 2", got)
	}

	cals := st.Calendars()
	if len(cals) != 2 {
		t.Fatalf("Calendars() = %d, want 2", len(cals))
	}
	ids := map[string]bool{}
	for _, c := range cals {
		if ids[c.ID] {
			t.Fatalf("duplicate collection ID across sources: %q", c.ID)
		}
		ids[c.ID] = true
		if c.Source != "alpha" && c.Source != "beta" {
			t.Fatalf("unexpected source %q on %q", c.Source, c.ID)
		}
	}

	// Each address book's contacts are reachable by its namespaced ID.
	for _, b := range st.AddressBooks() {
		cs := st.Contacts(b.ID)
		if len(cs) != 1 || cs[0].FN != "Ada Lovelace" {
			t.Fatalf("Contacts(%q) = %+v, want one 'Ada Lovelace'", b.ID, cs)
		}
	}

	// Occurrences merge across both sources.
	occs := st.Occurrences(model.DateRange{
		From: mustTime(t, "2026-06-01T00:00:00Z"),
		To:   mustTime(t, "2026-07-01T00:00:00Z"),
	}, nil)
	if len(occs) != 2 {
		t.Fatalf("Occurrences across both sources = %d, want 2", len(occs))
	}
}

// TestReloadRoutesToOwningSource confirms Reload refreshes only via the backend
// that owns the collection.
func TestReloadRoutesToOwningSource(t *testing.T) {
	aCal, aCon := writeSource(t, t.TempDir(), "work")
	st := New(LocalSource("alpha", "Alpha", aCal, aCon))
	if err := st.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := st.Calendars()[0].ID
	if err := st.Reload(context.Background(), id); err != nil {
		t.Fatalf("Reload(%q): %v", id, err)
	}
	// Reloading an unknown collection is a no-op, not an error.
	if err := st.Reload(context.Background(), "ghost/nope"); err != nil {
		t.Fatalf("Reload(unknown): %v", err)
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
