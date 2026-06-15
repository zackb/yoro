// Package ui implements Yoro's terminal interface: a top-level model that
// switches between a Calendar pane and a Contacts pane, both sharing vim
// navigation and a preview-follows-cursor layout inspired by yazi.
package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zackb/yoro/internal/store"
)

// Mode selects the active pane.
type Mode int

const (
	ModeCalendar Mode = iota
	ModeContacts
)

// pane is the shared behavior of the calendar and contacts views.
type pane interface {
	setSize(w, h int)
	Update(tea.Msg) (tea.Cmd, bool)
	View() string
	refresh()
	isSearching() bool
}

func (p *calendarPane) isSearching() bool { return false }
func (p *contactsPane) isSearching() bool { return p.searching }

type storeLoadedMsg struct{ err error }

// App is the root bubbletea model.
type App struct {
	store store.Store
	theme Theme
	keys  KeyMap

	mode          Mode
	width, height int

	cal *calendarPane
	con *contactsPane

	loading bool
	loadErr error
	spin    spinner.Model

	showHelp bool
	status   string
}

// New constructs the root model over a store.
func New(st store.Store) App {
	theme := DefaultTheme()
	keys := DefaultKeyMap()
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(theme.Accent)
	return App{
		store:   st,
		theme:   theme,
		keys:    keys,
		cal:     newCalendarPane(theme, keys, st),
		con:     newContactsPane(theme, keys, st),
		loading: true,
		spin:    sp,
	}
}

func (a App) Init() tea.Cmd {
	return tea.Batch(a.spin.Tick, a.load)
}

func (a App) load() tea.Msg {
	return storeLoadedMsg{err: a.store.Load(context.Background())}
}

func (a App) activePane() pane {
	if a.mode == ModeContacts {
		return a.con
	}
	return a.cal
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.layout()
		return a, nil

	case storeLoadedMsg:
		a.loading = false
		a.loadErr = msg.err
		if msg.err == nil {
			a.cal.refresh()
			a.con.refresh()
		}
		return a, nil

	case spinner.TickMsg:
		if a.loading {
			var cmd tea.Cmd
			a.spin, cmd = a.spin.Update(msg)
			return a, cmd
		}
		return a, nil

	case tea.KeyMsg:
		return a.handleKey(msg)
	}
	return a, nil
}

func (a App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// When a pane owns text input (search), route everything to it first.
	if a.activePane().isSearching() {
		cmd, _ := a.activePane().Update(msg)
		return a, cmd
	}

	if a.showHelp {
		if key.Matches(msg, a.keys.Help, a.keys.Escape, a.keys.Quit) {
			a.showHelp = false
		}
		return a, nil
	}

	switch {
	case key.Matches(msg, a.keys.Quit):
		return a, tea.Quit
	case key.Matches(msg, a.keys.Help):
		a.showHelp = true
		return a, nil
	case key.Matches(msg, a.keys.NextMode):
		a.mode = (a.mode + 1) % 2
		return a, nil
	case key.Matches(msg, a.keys.Calendar):
		a.mode = ModeCalendar
		return a, nil
	case key.Matches(msg, a.keys.Contacts):
		a.mode = ModeContacts
		return a, nil
	case key.Matches(msg, a.keys.Reload):
		a.loading = true
		return a, tea.Batch(a.spin.Tick, a.load)
	}

	cmd, _ := a.activePane().Update(msg)
	return a, cmd
}

// layout assigns sizes to the panes (full width, height minus the status bar).
func (a *App) layout() {
	ph := a.height - 1
	if ph < 1 {
		ph = 1
	}
	a.cal.setSize(a.width, ph)
	a.con.setSize(a.width, ph)
}

func (a App) View() string {
	if a.width == 0 {
		return ""
	}
	if a.loading {
		body := fmt.Sprintf("%s loading calendars and contacts…", a.spin.View())
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center,
			a.theme.Title.Render("Yoro")+"\n\n"+body)
	}
	if a.loadErr != nil {
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center,
			a.theme.Title.Render("Yoro")+"\n\nfailed to load: "+a.loadErr.Error())
	}
	if a.showHelp {
		return a.helpView()
	}

	body := a.activePane().View()
	return body + "\n" + a.statusBar()
}

func (a App) statusBar() string {
	calChip := " 1 calendar "
	conChip := " 2 contacts "
	if a.mode == ModeCalendar {
		calChip = a.theme.StatusMode.Render(IconCalendar + " calendar")
		conChip = a.theme.StatusBar.Render("  contacts")
	} else {
		calChip = a.theme.StatusBar.Render("  calendar")
		conChip = a.theme.StatusMode.Render(IconContacts + " contacts")
	}

	status := a.paneStatus()
	hints := a.theme.StatusBar.Render("h/j/k/l move · / search · tab switch · ? help · q quit")

	left := calChip + " " + conChip
	if status != "" {
		left += "  " + a.theme.StatusKey.Render(status)
	}
	gap := a.width - lipgloss.Width(left) - lipgloss.Width(hints)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + hints
}

func (a App) paneStatus() string {
	if a.mode == ModeContacts {
		return a.con.status
	}
	return a.cal.status
}

func (a App) helpView() string {
	lines := []string{
		a.theme.Title.Render("Yoro — keybindings"),
		"",
		section(a.theme, "Global"),
		row(a.theme, "tab / 1 / 2", "switch calendar / contacts"),
		row(a.theme, "R", "reload from disk"),
		row(a.theme, "? ", "toggle this help"),
		row(a.theme, "q / ctrl+c", "quit"),
		"",
		section(a.theme, "Navigation"),
		row(a.theme, "h / l", "focus column left / right"),
		row(a.theme, "j / k", "down / up"),
		row(a.theme, "gg / G", "top / bottom"),
		row(a.theme, "ctrl+d / ctrl+u", "half-page down / up"),
		row(a.theme, "/ ", "search in pane"),
		"",
		section(a.theme, "Calendar"),
		row(a.theme, "t", "jump to today"),
		row(a.theme, "} / {", "next / previous day"),
		row(a.theme, "J / K", "next / previous month"),
		row(a.theme, "space", "toggle highlighted collection"),
		row(a.theme, "T", "toggle tasks"),
		"",
		section(a.theme, "Contacts"),
		row(a.theme, "y", "yank email/phone to clipboard"),
		"",
		a.theme.Help.Render("press ? or esc to close"),
	}
	content := strings.Join(lines, "\n")
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center,
		a.theme.Column("", content, lipgloss.Width(content)+4, len(lines)+2, true))
}

func section(t Theme, s string) string { return t.ColTitle.Render(s) }
func row(t Theme, keys, desc string) string {
	return t.StatusKey.Render(PadRight(keys, 18)) + t.Value.Render(desc)
}
