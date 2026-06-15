// Package config loads Yoro's configuration: an ordered list of sources, each a
// local vdir tree (vdirsyncer/khal layout) or a remote CalDAV/CardDAV account.
package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Source types.
const (
	SourceLocal = "local"
	SourceDAV   = "dav"
)

// Source is one configured backend. Local sources use Calendars/Contacts; DAV
// sources use URL/Username and one of Password/PasswordCommand.
type Source struct {
	Name string `toml:"name"`
	Type string `toml:"type"` // "local" | "dav"

	// Local sources: roots containing per-collection directories.
	Calendars string `toml:"calendars"`
	Contacts  string `toml:"contacts"`

	// DAV sources.
	URL             string `toml:"url"`
	Username        string `toml:"username"`
	Password        string `toml:"password"`
	PasswordCommand string `toml:"password_command"`
}

// Secret resolves the DAV password, preferring PasswordCommand (run via the
// shell, trailing newline trimmed) so plaintext need not live in the config.
func (s Source) Secret() (string, error) {
	if s.PasswordCommand != "" {
		out, err := exec.Command("sh", "-c", s.PasswordCommand).Output()
		if err != nil {
			return "", fmt.Errorf("password_command: %w", err)
		}
		return strings.TrimRight(string(out), "\r\n"), nil
	}
	return s.Password, nil
}

// Config is the resolved configuration.
type Config struct {
	Sources []Source `toml:"sources"`
}

// Default returns a single local source at the standard vdirsyncer/khal
// locations under $XDG_DATA_HOME (falling back to ~/.local/share).
func Default() Config {
	data := dataHome()
	return Config{Sources: []Source{{
		Name:      "local",
		Type:      SourceLocal,
		Calendars: filepath.Join(data, "calendars"),
		Contacts:  filepath.Join(data, "contacts"),
	}}}
}

// Load reads $XDG_CONFIG_HOME/yoro/config.toml. A missing file (or one with no
// sources) yields Default(). Paths beginning with ~ are expanded.
func Load() (Config, error) {
	path := filepath.Join(configHome(), "yoro", "config.toml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(cfg.Sources) == 0 {
		return Default(), nil
	}
	for i := range cfg.Sources {
		s := &cfg.Sources[i]
		if s.Type == "" {
			s.Type = SourceLocal
		}
		if s.Name == "" {
			s.Name = s.Type
		}
		s.Calendars = expandPath(s.Calendars)
		s.Contacts = expandPath(s.Contacts)
	}
	return cfg, nil
}

// dataHome resolves $XDG_DATA_HOME or ~/.local/share.
func dataHome() string { return xdgHome("XDG_DATA_HOME", filepath.Join(".local", "share")) }

// configHome resolves $XDG_CONFIG_HOME or ~/.config.
func configHome() string { return xdgHome("XDG_CONFIG_HOME", ".config") }

// xdgHome returns the value of env, or fallbackSub under the user's home.
func xdgHome(env, fallbackSub string) string {
	if d := os.Getenv(env); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fallbackSub
	}
	return filepath.Join(home, fallbackSub)
}

// expandPath expands a leading ~ to the user's home directory.
func expandPath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
