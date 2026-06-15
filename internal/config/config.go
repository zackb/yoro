// Package config loads Yoro's configuration and discovers on-disk
// calendar collections and address books in the vdirsyncer/khal layout.
package config

import (
	"os"
	"path/filepath"
)

// Config holds the resolved runtime configuration.
type Config struct {
	// CalendarsDir is the root containing account/collection directories of .ics files.
	CalendarsDir string
	// ContactsDir is the root containing account/addressbook directories of .vcf files.
	ContactsDir string
}

// Default returns a Config pointing at the standard vdirsyncer/khal locations
// under $XDG_DATA_HOME (falling back to ~/.local/share).
func Default() Config {
	data := dataHome()
	return Config{
		CalendarsDir: filepath.Join(data, "calendars"),
		ContactsDir:  filepath.Join(data, "contacts"),
	}
}

// dataHome resolves $XDG_DATA_HOME or ~/.local/share.
func dataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".local/share"
	}
	return filepath.Join(home, ".local", "share")
}
