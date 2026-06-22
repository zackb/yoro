package ui

import (
	"context"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zackb/yoro/internal/config"
	"github.com/zackb/yoro/internal/store"
)

// localSource builds a single local source from the default config locations.
func localSource() store.Source {
	s := config.Default().Sources[0]
	return store.LocalSource(s.Name, s.Name, s.Calendars, s.Contacts)
}

// TestRenderSmoke loads the real local data and prints rendered frames for both
// modes. Run with: go test ./internal/ui -run RenderSmoke -v
func TestRenderSmoke(t *testing.T) {
	st := store.New(localSource())
	if err := st.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	var m tea.Model = New(st)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 150, Height: 42})
	m, _ = m.Update(storeLoadedMsg{})

	frame := func(label string) {
		fmt.Printf("\n===== %s =====\n%s\n", label, m.View())
	}
	frame("CALENDAR")

	// Month grid.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	frame("CALENDAR month grid")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})

	// Switch to contacts.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	frame("CONTACTS")

	// Move down a few contacts.
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	frame("CONTACTS after 5x j")
}

// TestInteractions drives many keypresses across both modes to ensure none of
// the index math panics and View always renders.
func TestInteractions(t *testing.T) {
	st := store.New(localSource())
	if err := st.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	var m tea.Model = New(st)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(storeLoadedMsg{})

	press := func(keys ...string) {
		for _, k := range keys {
			var msg tea.KeyMsg
			switch k {
			case "tab":
				msg = tea.KeyMsg{Type: tea.KeyTab}
			case "enter":
				msg = tea.KeyMsg{Type: tea.KeyEnter}
			case "esc":
				msg = tea.KeyMsg{Type: tea.KeyEscape}
			case "ctrl+d":
				msg = tea.KeyMsg{Type: tea.KeyCtrlD}
			case "ctrl+u":
				msg = tea.KeyMsg{Type: tea.KeyCtrlU}
			default:
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
			}
			m, _ = m.Update(msg)
			if m.View() == "" {
				t.Fatalf("empty view after key %q", k)
			}
		}
	}

	// Calendar: focus sidebar, toggle collections, jump around, deep scroll.
	press("1", "h", "j", "j", " ", " ", "l")
	press("t", "}", "}", "}", "{", "G", "g", "g")
	for i := 0; i < 80; i++ {
		press("j")
	}
	for i := 0; i < 120; i++ {
		press("k")
	}
	press("J", "J", "J", "K", "K", "T", "T", "?", "?")

	// Month grid: toggle in, navigate by day/week/month, drill back via enter.
	press("m")
	press("l", "l", "h", "j", "j", "k", "J", "K", "t")
	for i := 0; i < 40; i++ {
		press("l")
	}
	press("enter")    // drills back to the agenda list at the selected day
	press("m", "m")   // toggle in and back out
	press("m", "esc") // esc while in grid should not panic

	// Contacts: search, navigate, clear, yank.
	press("2", "/", "b", "a", "i", "l", "enter")
	press("j", "k", "y", "esc")
	press("/", "z", "z", "z", "z", "z", "enter") // no matches
	press("esc", "G", "g")

	// Calendar: search, navigate, clear; search then toggle month grid.
	press("1", "/", "a", "enter")
	press("j", "k", "esc")
	press("/", "z", "z", "z", "z", "z", "enter") // no matches
	press("esc")
	press("/", "a", "m", "m") // month grid honors the active filter
	press("esc")
}

// TestCalendarSearch asserts that the calendar search narrows the agenda and that
// clearing it restores the full list.
func TestCalendarSearch(t *testing.T) {
	st := store.New(localSource())
	if err := st.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	var m tea.Model = New(st)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(storeLoadedMsg{})

	key := func(k tea.KeyMsg) { m, _ = m.Update(k) }
	runes := func(s string) {
		for _, r := range s {
			key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}

	cal := m.(App).cal
	full := len(cal.selRows)

	// A query that cannot match should empty the agenda.
	key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !cal.searching {
		t.Fatal("expected calendar to enter search mode after /")
	}
	runes("zqxjkv")
	key(tea.KeyMsg{Type: tea.KeyEnter})
	if cal.searching {
		t.Fatal("enter should commit the query and leave search mode")
	}
	if got := len(cal.selRows); got != 0 {
		t.Fatalf("no-match query should empty agenda, got %d rows", got)
	}

	// Esc in normal mode clears the query and restores the full list.
	key(tea.KeyMsg{Type: tea.KeyEscape})
	if cal.query != "" {
		t.Fatalf("esc should clear query, still %q", cal.query)
	}
	if got := len(cal.selRows); got != full {
		t.Fatalf("clearing search should restore %d rows, got %d", full, got)
	}
}
