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

	create   *createForm // non-nil while the create overlay is open
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
	// The create overlay is modal: it captures all keys until submit/cancel.
	if a.create != nil {
		return a.handleCreate(msg)
	}

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
	case key.Matches(msg, a.keys.Create):
		a.openCreate()
		return a, nil
	}

	cmd, _ := a.activePane().Update(msg)
	return a, cmd
}

// openCreate opens the create overlay targeting the current pane's selected
// collection, or sets a pane status if there's nothing to target.
func (a *App) openCreate() {
	switch a.mode {
	case ModeCalendar:
		col, ok := a.cal.selectedCalendar()
		if !ok {
			a.cal.status = "no calendar selected"
			return
		}
		a.create = newEventForm(a.theme, col, a.sourceName(col.Source))
	case ModeContacts:
		col, ok := a.con.selectedBook()
		if !ok {
			a.con.status = "no address book selected"
			return
		}
		a.create = newContactForm(a.theme, col, a.sourceName(col.Source))
	}
}

// handleCreate routes a key to the open create overlay, persisting on submit.
func (a App) handleCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	submitted, cancelled, cmd := a.create.update(msg)
	switch {
	case cancelled:
		a.create = nil
	case submitted:
		if err := a.submitCreate(); err != nil {
			a.create.err = err.Error()
		} else {
			a.create = nil
		}
	}
	return a, cmd
}

// submitCreate builds the model from the form, persists it via the store, and
// refreshes the affected pane. A returned error is shown in the form.
func (a App) submitCreate() error {
	ctx := context.Background()
	switch a.create.domain {
	case ModeCalendar:
		e, err := a.create.buildEvent()
		if err != nil {
			return err
		}
		if err := a.store.CreateEvent(ctx, a.create.target.ID, e); err != nil {
			return err
		}
		a.cal.refresh()
		a.cal.status = "created event"
	case ModeContacts:
		c, err := a.create.buildContact()
		if err != nil {
			return err
		}
		if err := a.store.CreateContact(ctx, a.create.target.ID, c); err != nil {
			return err
		}
		a.con.refresh()
		a.con.status = "created contact"
	}
	return nil
}

// sourceName resolves a source id to its display name.
func (a App) sourceName(id string) string {
	for _, s := range a.store.Sources() {
		if s.ID == id {
			return s.Name
		}
	}
	return id
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
	if a.create != nil {
		return a.create.view(a.width, a.height)
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
	hintText := "h/j/k/l move · / search · tab switch · ? help · q quit"
	if a.mode == ModeContacts && len(a.con.sources) > 1 {
		hintText = "s source · " + hintText
	}
	hints := a.theme.StatusBar.Render(hintText)

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
		row(a.theme, "a", "new event / contact"),
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
		row(a.theme, "s", "switch source (local / DAV)"),
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
