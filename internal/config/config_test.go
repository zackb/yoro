package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDefault builds the single implicit local source under $XDG_DATA_HOME.
func TestDefault(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	cfg := Default()
	if len(cfg.Sources) != 1 {
		t.Fatalf("want 1 default source, got %d", len(cfg.Sources))
	}
	s := cfg.Sources[0]
	if s.Type != SourceLocal {
		t.Errorf("default type = %q, want %q", s.Type, SourceLocal)
	}
	if s.Calendars != "/data/calendars" || s.Contacts != "/data/contacts" {
		t.Errorf("default paths = %q / %q", s.Calendars, s.Contacts)
	}
}

// TestLoadMissingFile falls back to Default when no config file exists.
func TestLoadMissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // empty dir: no config.toml
	t.Setenv("XDG_DATA_HOME", "/data")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].Name != "local" {
		t.Fatalf("missing file should yield Default, got %+v", cfg.Sources)
	}
}

// TestLoadEmptySourcesFallsBack treats a config with no sources as Default.
func TestLoadEmptySourcesFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", "/data")
	writeConfig(t, dir, "# no sources\n")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].Type != SourceLocal {
		t.Fatalf("empty sources should yield Default, got %+v", cfg.Sources)
	}
}

// TestLoadParsesAndDefaults parses sources, applies type/name defaults, and
// expands ~ in local paths.
func TestLoadParsesAndDefaults(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", home)
	writeConfig(t, dir, `
[[sources]]
calendars = "~/cal"
contacts = "~/con"

[[sources]]
name = "remote"
type = "dav"
url = "https://dav.example.com/"
username = "me"
`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 2 {
		t.Fatalf("want 2 sources, got %d", len(cfg.Sources))
	}

	local := cfg.Sources[0]
	if local.Type != SourceLocal { // type defaulted
		t.Errorf("local type = %q, want %q", local.Type, SourceLocal)
	}
	if local.Name != SourceLocal { // name defaulted to type
		t.Errorf("local name = %q, want %q", local.Name, SourceLocal)
	}
	if want := filepath.Join(home, "cal"); local.Calendars != want {
		t.Errorf("~ not expanded: %q, want %q", local.Calendars, want)
	}

	if cfg.Sources[1].Type != SourceDAV {
		t.Errorf("remote type = %q, want %q", cfg.Sources[1].Type, SourceDAV)
	}
}

func TestSecretInlinePassword(t *testing.T) {
	got, err := Source{Password: "hunter2"}.Secret()
	if err != nil || got != "hunter2" {
		t.Fatalf("Secret() = %q, %v; want hunter2, nil", got, err)
	}
}

func TestSecretCommandTrimsNewline(t *testing.T) {
	got, err := Source{PasswordCommand: "printf 'hunter2\\n'"}.Secret()
	if err != nil {
		t.Fatal(err)
	}
	if got != "hunter2" {
		t.Fatalf("Secret() = %q, want hunter2 (trailing newline trimmed)", got)
	}
}

func TestSecretCommandPreferredOverPassword(t *testing.T) {
	got, err := Source{Password: "inline", PasswordCommand: "printf cmd"}.Secret()
	if err != nil {
		t.Fatal(err)
	}
	if got != "cmd" {
		t.Fatalf("Secret() = %q, want cmd (command preferred)", got)
	}
}

func TestSecretCommandError(t *testing.T) {
	if _, err := (Source{PasswordCommand: "exit 1"}).Secret(); err == nil {
		t.Fatal("expected error from failing password_command")
	}
}

func TestExpandPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := map[string]string{
		"~":         home,
		"~/sub":     filepath.Join(home, "sub"),
		"/abs/path": "/abs/path",
		"rel":       "rel",
	}
	for in, want := range cases {
		if got := expandPath(in); got != want {
			t.Errorf("expandPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func writeConfig(t *testing.T, configHome, body string) {
	t.Helper()
	dir := filepath.Join(configHome, "yoro")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
