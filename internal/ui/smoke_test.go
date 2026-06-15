package ui

import (
	"context"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zackb/yoro/internal/config"
	"github.com/zackb/yoro/internal/store"
)

// TestRenderSmoke loads the real local data and prints rendered frames for both
// modes. Run with: go test ./internal/ui -run RenderSmoke -v
func TestRenderSmoke(t *testing.T) {
	st := store.New(store.NewLocal(config.Default()))
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
	st := store.New(store.NewLocal(config.Default()))
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

	// Contacts: search, navigate, clear, yank.
	press("2", "/", "b", "a", "i", "l", "enter")
	press("j", "k", "y", "esc")
	press("/", "z", "z", "z", "z", "z", "enter") // no matches
	press("esc", "G", "g")
}
