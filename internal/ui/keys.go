package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap holds all keybindings. Vim motions are shared by both panes; calendar-
// and contacts-specific keys are namespaced below.
type KeyMap struct {
	// Global
	Quit     key.Binding
	Help     key.Binding
	NextMode key.Binding
	Calendar key.Binding
	Contacts key.Binding
	Reload   key.Binding
	Create   key.Binding
	Edit     key.Binding

	// Navigation
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Top      key.Binding
	Bottom   key.Binding
	HalfDown key.Binding
	HalfUp   key.Binding
	PageDown key.Binding
	PageUp   key.Binding

	// Search
	Search    key.Binding
	NextMatch key.Binding
	PrevMatch key.Binding
	Escape    key.Binding

	// Calendar
	Today     key.Binding
	NextDay   key.Binding
	PrevDay   key.Binding
	NextMonth key.Binding
	PrevMonth key.Binding
	Toggle    key.Binding
	Tasks     key.Binding

	// Contacts
	Yank         key.Binding
	SwitchSource key.Binding
}

// DefaultKeyMap returns the default bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		NextMode: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch mode")),
		Calendar: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "calendar")),
		Contacts: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "contacts")),
		Reload:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload")),
		Create:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "new item")),
		Edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit item")),

		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "down")),
		Left:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h", "left")),
		Right:    key.NewBinding(key.WithKeys("l", "right", "enter"), key.WithHelp("l", "right")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("gg", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		HalfDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half-page down")),
		HalfUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half-page up")),
		PageDown: key.NewBinding(key.WithKeys("ctrl+f", "pgdown"), key.WithHelp("ctrl+f", "page down")),
		PageUp:   key.NewBinding(key.WithKeys("ctrl+b", "pgup"), key.WithHelp("ctrl+b", "page up")),

		Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		NextMatch: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
		PrevMatch: key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
		Escape:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear")),

		Today:     key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "today")),
		NextDay:   key.NewBinding(key.WithKeys("}"), key.WithHelp("}", "next day")),
		PrevDay:   key.NewBinding(key.WithKeys("{"), key.WithHelp("{", "prev day")),
		NextMonth: key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "next month")),
		PrevMonth: key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "prev month")),
		Toggle:    key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle collection")),
		Tasks:     key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "toggle tasks")),

		Yank:         key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yank")),
		SwitchSource: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "switch source")),
	}
}
