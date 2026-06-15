package dav

import (
	"context"
	"os"
	"testing"

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
