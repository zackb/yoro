// Command yoro is a yazi-inspired terminal UI for browsing calendars and
// contacts from local vdir trees and remote CalDAV/CardDAV servers.
package main

import (
	"flag"
	"fmt"
	"os"
	_ "time/tzdata" // embed the IANA tz database for a self-contained static binary

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zackb/yoro/internal/config"
	"github.com/zackb/yoro/internal/store"
	"github.com/zackb/yoro/internal/ui"
	"github.com/zackb/yoro/internal/version"
)

func main() {
	var (
		showVersion = flag.Bool("version", false, "print version and exit")
		calendars   = flag.String("calendars", "", "override the default local source's calendars directory")
		contacts    = flag.String("contacts", "", "override the default local source's contacts directory")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("yoro", version.String())
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "yoro:", err)
		os.Exit(1)
	}
	applyDirFlags(&cfg, *calendars, *contacts)

	sources, err := buildSources(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "yoro:", err)
		os.Exit(1)
	}

	st := store.New(sources...)
	app := ui.New(st)

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "yoro:", err)
		os.Exit(1)
	}
}

// applyDirFlags lets the --calendars/--contacts flags override the first local
// source, preserving the pre-config-file behavior.
func applyDirFlags(cfg *config.Config, calendars, contacts string) {
	if calendars == "" && contacts == "" {
		return
	}
	for i := range cfg.Sources {
		if cfg.Sources[i].Type != config.SourceLocal {
			continue
		}
		if calendars != "" {
			cfg.Sources[i].Calendars = calendars
		}
		if contacts != "" {
			cfg.Sources[i].Contacts = contacts
		}
		return
	}
}

// buildSources turns config sources into store sources, constructing a backend
// per source. No network I/O happens here: DAV backends connect lazily during
// Store.Load (off the UI thread), so the splash appears immediately and an
// unreachable server surfaces as a non-fatal warning rather than a startup hang.
func buildSources(cfg config.Config) ([]store.Source, error) {
	var out []store.Source
	for _, s := range cfg.Sources {
		switch s.Type {
		case config.SourceLocal:
			out = append(out, store.LocalSource(s.Name, s.Name, s.Calendars, s.Contacts))
		case config.SourceDAV:
			secret, err := s.Secret()
			if err != nil {
				return nil, fmt.Errorf("source %q: %w", s.Name, err)
			}
			out = append(out, store.DAVSource(s.Name, s.Name, s.URL, s.Username, secret))
		default:
			return nil, fmt.Errorf("source %q: unknown type %q", s.Name, s.Type)
		}
	}
	return out, nil
}
