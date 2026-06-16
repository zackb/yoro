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

// deleteDoneMsg reports the result of an async delete, so the (possibly slow)
// DAV round-trip never blocks the UI thread.
type deleteDoneMsg struct {
	domain Mode
	err    error
}

// saveDoneMsg reports the result of an async create/edit save, for the same
// reason as deleteDoneMsg.
type saveDoneMsg struct {
	domain  Mode
	editing bool
	err     error
}

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

	create    *createForm    // non-nil while the create overlay is open
	confirm   *confirmPrompt // non-nil while the delete confirmation is open
	busy      bool           // an async mutation (delete) is in flight
	busyLabel string         // spinner caption while busy
	showHelp  bool
	status    string
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
		if a.loading || a.busy {
			var cmd tea.Cmd
			a.spin, cmd = a.spin.Update(msg)
			return a, cmd
		}
		return a, nil

	case deleteDoneMsg:
		a.busy = false
		switch msg.domain {
		case ModeCalendar:
			if msg.err != nil {
				a.cal.status = "delete failed: " + msg.err.Error()
			} else {
				a.cal.refresh()
				a.cal.status = "deleted event"
			}
		case ModeContacts:
			if msg.err != nil {
				a.con.status = "delete failed: " + msg.err.Error()
			} else {
				a.con.refresh()
				a.con.status = "deleted contact"
			}
		}
		return a, nil

	case saveDoneMsg:
		a.busy = false
		switch msg.domain {
		case ModeCalendar:
			if msg.err != nil {
				a.cal.status = "save failed: " + msg.err.Error()
			} else {
				a.cal.refresh()
				a.cal.status = verbed(msg.editing, "event")
			}
		case ModeContacts:
			if msg.err != nil {
				a.con.status = "save failed: " + msg.err.Error()
			} else {
				a.con.refresh()
				a.con.status = verbed(msg.editing, "contact")
			}
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

	// The delete confirmation is modal: it captures all keys until answered.
	if a.confirm != nil {
		return a.handleConfirm(msg)
	}

	// While an async mutation is in flight, swallow keys so a second action
	// can't race the first.
	if a.busy {
		return a, nil
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
	case key.Matches(msg, a.keys.Edit):
		a.openEdit()
		return a, nil
	case key.Matches(msg, a.keys.Delete):
		a.openDelete()
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

// openEdit opens the form pre-filled from the selected item. Recurring events
// are refused for now (editing the master vs a single instance is a later
// milestone).
func (a *App) openEdit() {
	switch a.mode {
	case ModeCalendar:
		occ, ok := a.cal.selectedOcc()
		if !ok || occ.Event == nil {
			a.cal.status = "no event selected"
			return
		}
		if occ.Event.Recurring() {
			a.cal.status = "editing recurring events not supported yet"
			return
		}
		col := a.cal.colByID[occ.CollectionID]
		a.create = newEditEventForm(a.theme, col, a.sourceName(col.Source), *occ.Event)
	case ModeContacts:
		c, ok := a.con.selected()
		if !ok {
			a.con.status = "no contact selected"
			return
		}
		col, _ := a.con.selectedBook()
		a.create = newEditContactForm(a.theme, col, a.sourceName(col.Source), c)
	}
}

// openDelete opens the confirmation overlay for the selected item. Recurring
// events are refused for now (deleting the master vs a single instance is a
// later milestone), mirroring openEdit.
func (a *App) openDelete() {
	switch a.mode {
	case ModeCalendar:
		occ, ok := a.cal.selectedOcc()
		if !ok || occ.Event == nil {
			a.cal.status = "no event selected"
			return
		}
		if occ.Event.Recurring() {
			a.cal.status = "deleting recurring events not supported yet"
			return
		}
		a.confirm = &confirmPrompt{
			theme:   a.theme,
			title:   "Delete event",
			message: "Delete “" + occ.Summary + "”?",
			domain:  ModeCalendar,
			colID:   occ.CollectionID,
			path:    occ.Event.Path,
		}
	case ModeContacts:
		c, ok := a.con.selected()
		if !ok {
			a.con.status = "no contact selected"
			return
		}
		col, _ := a.con.selectedBook()
		a.confirm = &confirmPrompt{
			theme:   a.theme,
			title:   "Delete contact",
			message: "Delete “" + c.DisplayName() + "”?",
			domain:  ModeContacts,
			colID:   col.ID,
			path:    c.Path,
		}
	}
}

// handleConfirm routes a key to the open delete confirmation, kicking off the
// async deletion on y and dismissing on n/esc.
func (a App) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		c := a.confirm
		a.confirm = nil
		a.busy = true
		a.busyLabel = "deleting " + noun(c.domain)
		return a, tea.Batch(a.spin.Tick, a.deleteCmd(c))
	case "n", "N", "esc", "q":
		a.confirm = nil
	}
	return a, nil
}

// deleteCmd performs the store delete off the UI thread (the DAV round-trip can
// take a moment) and reports completion via deleteDoneMsg.
func (a App) deleteCmd(c *confirmPrompt) tea.Cmd {
	st := a.store
	return func() tea.Msg {
		ctx := context.Background()
		var err error
		switch c.domain {
		case ModeCalendar:
			err = st.DeleteEvent(ctx, c.colID, c.path)
		case ModeContacts:
			err = st.DeleteContact(ctx, c.colID, c.path)
		}
		return deleteDoneMsg{domain: c.domain, err: err}
	}
}

func noun(m Mode) string {
	if m == ModeContacts {
		return "contact"
	}
	return "event"
}

// handleCreate routes a key to the open create/edit overlay. On submit it
// validates synchronously (so the form stays open to show input errors), then
// kicks off the actual store write off the UI thread.
func (a App) handleCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	submitted, cancelled, cmd := a.create.update(msg)
	switch {
	case cancelled:
		a.create = nil
	case submitted:
		saveCmd, err := a.buildSaveCmd()
		if err != nil {
			a.create.err = err.Error()
			return a, cmd
		}
		domain := a.create.domain
		a.create = nil
		a.busy = true
		a.busyLabel = "saving " + noun(domain)
		return a, tea.Batch(a.spin.Tick, saveCmd)
	}
	return a, cmd
}

// buildSaveCmd validates the form into a model and returns a command that
// persists it (the slow DAV PUT) off the UI thread, reporting via saveDoneMsg. A
// validation error is returned synchronously so the form can stay open to show it.
func (a App) buildSaveCmd() (tea.Cmd, error) {
	st := a.store
	editing := a.create.editing
	domain := a.create.domain
	colID := a.create.target.ID
	switch domain {
	case ModeCalendar:
		e, err := a.create.buildEvent()
		if err != nil {
			return nil, err
		}
		return func() tea.Msg {
			ctx := context.Background()
			if editing {
				err = st.UpdateEvent(ctx, colID, e)
			} else {
				err = st.CreateEvent(ctx, colID, e)
			}
			return saveDoneMsg{domain: domain, editing: editing, err: err}
		}, nil
	case ModeContacts:
		c, err := a.create.buildContact()
		if err != nil {
			return nil, err
		}
		return func() tea.Msg {
			ctx := context.Background()
			if editing {
				err = st.UpdateContact(ctx, colID, c)
			} else {
				err = st.CreateContact(ctx, colID, c)
			}
			return saveDoneMsg{domain: domain, editing: editing, err: err}
		}, nil
	}
	return nil, nil
}

func verbed(editing bool, noun string) string {
	if editing {
		return "updated " + noun
	}
	return "created " + noun
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
	if a.confirm != nil {
		return a.confirm.view(a.width, a.height)
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
	if a.busy {
		status = a.spin.View() + " " + a.busyLabel + "…"
	}
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
		row(a.theme, "e", "edit selected event / contact"),
		row(a.theme, "d", "delete selected event / contact"),
		row(a.theme, "R", "reload from disk"),
		row(a.theme, "? ", "toggle this help"),
		row(a.theme, "q / ctrl+c", "quit"),
		"",
		section(a.theme, "Navigation"),
		row(a.theme, "h / l", "focus column left / right"),
		row(a.theme, "j / k", "down / up"),
		row(a.theme, "gg / G", "top / bottom"),
		row(a.theme, "ctrl+d / ctrl+u", "half-page down / up"),
		row(a.theme, "ctrl+f / ctrl+b", "page down / up"),
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
