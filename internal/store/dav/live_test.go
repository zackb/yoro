package dav

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zackb/yoro/internal/config"
	"github.com/zackb/yoro/internal/model"
)

// TestLive exercises the DAV backend against a real server using the first dav
// source in the user's config. Opt in with YORO_LIVE_DAV=1; skipped otherwise.
func TestLive(t *testing.T) {
	if os.Getenv("YORO_LIVE_DAV") == "" {
		t.Skip("set YORO_LIVE_DAV=1 to run against the configured DAV server")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	var src config.Source
	for _, s := range cfg.Sources {
		if s.Type == config.SourceDAV {
			src = s
			break
		}
	}
	if src.Name == "" {
		t.Skip("no dav source in config")
	}
	secret, err := src.Secret()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	d, err := New(ctx, src.Name, src.URL, src.Username, secret)
	if err != nil {
		t.Fatalf("connect %s: %v", src.URL, err)
	}

	cols, err := d.Collections(ctx)
	if err != nil {
		t.Fatalf("Collections: %v", err)
	}
	t.Logf("discovered %d collections at %s", len(cols), src.URL)
	for _, c := range cols {
		switch c.Kind {
		case model.KindCalendar:
			ev, _ := d.Events(ctx, c.ID)
			td, _ := d.Todos(ctx, c.ID)
			t.Logf("  [cal]  %-24q events=%d todos=%d", c.Name, len(ev), len(td))
		case model.KindAddressBook:
			cs, _ := d.Contacts(ctx, c.ID)
			t.Logf("  [book] %-24q contacts=%d", c.Name, len(cs))
		}
	}
}

// TestLiveCreate creates a real event and contact on the configured DAV server,
// then re-reads to confirm they persisted with an ETag. Opt in with
// YORO_LIVE_DAV=1. NOTE: this leaves test data on the server (delete is not yet
// implemented).
func TestLiveCreate(t *testing.T) {
	if os.Getenv("YORO_LIVE_DAV") == "" {
		t.Skip("set YORO_LIVE_DAV=1 to run against the configured DAV server")
	}
	d := liveBackend(t)
	ctx := context.Background()
	cols, err := d.Collections(ctx)
	if err != nil {
		t.Fatal(err)
	}

	stamp := time.Now().Format("15:04:05")
	if cal := firstOfKind(cols, model.KindCalendar); cal.ID != "" {
		uid := uuid.NewString()
		summary := "yoro test " + stamp
		start := time.Now().Add(24 * time.Hour).Truncate(time.Hour)
		if err := d.PutEvent(ctx, cal.ID, model.Event{
			UID: uid, Summary: summary, Start: start, End: start.Add(time.Hour),
		}); err != nil {
			t.Fatalf("PutEvent into %q: %v", cal.Name, err)
		}
		evs, err := d.Events(ctx, cal.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !eventPresent(evs, uid, summary) {
			t.Fatalf("created event %q not found after re-read", summary)
		}
		t.Logf("created event %q in %q (%d events now)", summary, cal.Name, len(evs))
	}

	if book := firstOfKind(cols, model.KindAddressBook); book.ID != "" {
		uid := uuid.NewString()
		fn := "Yoro Test " + stamp
		if err := d.PutContact(ctx, book.ID, model.Contact{
			UID: uid, FN: fn, Emails: []model.TypedValue{{Value: "yoro@example.com"}},
		}); err != nil {
			t.Fatalf("PutContact into %q: %v", book.Name, err)
		}
		cs, err := d.Contacts(ctx, book.ID)
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, c := range cs {
			if c.UID == uid && c.FN == fn {
				found = true
			}
		}
		if !found {
			t.Fatalf("created contact %q not found after re-read", fn)
		}
		t.Logf("created contact %q in %q (%d contacts now)", fn, book.Name, len(cs))
	}
}

// liveBackend connects to the first dav source in the user's config.
func liveBackend(t *testing.T) *DAV {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	var src config.Source
	for _, s := range cfg.Sources {
		if s.Type == config.SourceDAV {
			src = s
			break
		}
	}
	if src.Name == "" {
		t.Skip("no dav source in config")
	}
	secret, err := src.Secret()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(context.Background(), src.Name, src.URL, src.Username, secret)
	if err != nil {
		t.Fatalf("connect %s: %v", src.URL, err)
	}
	return d
}

func firstOfKind(cols []model.Collection, kind model.Kind) model.Collection {
	for _, c := range cols {
		if c.Kind == kind {
			return c
		}
	}
	return model.Collection{}
}

func eventPresent(evs []model.Event, uid, summary string) bool {
	for _, e := range evs {
		if e.UID == uid && e.Summary == summary && e.ETag != "" {
			return true
		}
	}
	return false
}
