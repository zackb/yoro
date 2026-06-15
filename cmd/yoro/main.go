// Command yoro is a yazi-inspired terminal UI for browsing local calendars and
// contacts in the vdirsyncer/khal layout.
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
		calendars   = flag.String("calendars", "", "override calendars directory")
		contacts    = flag.String("contacts", "", "override contacts directory")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("yoro", version.String())
		return
	}

	cfg := config.Default()
	if *calendars != "" {
		cfg.CalendarsDir = *calendars
	}
	if *contacts != "" {
		cfg.ContactsDir = *contacts
	}

	st := store.New(store.NewLocal(cfg))
	app := ui.New(st)

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "yoro:", err)
		os.Exit(1)
	}
}
