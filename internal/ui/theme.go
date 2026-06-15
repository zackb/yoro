package ui

import "github.com/charmbracelet/lipgloss"

// Icons are nerd-font glyphs used throughout the UI. They degrade to mojibake
// on terminals without a nerd font, but layout is unaffected.
const (
	IconCalendar = "" // nf-md-calendar
	IconContacts = "" // nf-md-account_box
	IconEvent    = "" // nf-md-calendar_blank
	IconAllDay   = "" // nf-md-white_balance_sunny
	IconTask     = "" // nf-md-checkbox_marked_outline
	IconClock    = "" // nf-md-clock_outline
	IconLocation = "" // nf-md-map_marker
	IconPerson   = "" // nf-md-account
	IconEmail    = "" // nf-md-email
	IconPhone    = "" // nf-md-phone
	IconCake     = "" // nf-md-cake_variant
	IconOrg      = "" // nf-md-office_building
	IconNote     = "" // nf-md-note_text
	IconDot      = "●"
	IconCheckOn  = ""
	IconCheckOff = ""
	IconSearch   = ""
	IconRepeat   = "" // nf-md-repeat
	IconCloud    = "" // nf-md-cloud_outline — a remote (DAV) source
	IconDisk     = "" // nf-md-folder — a local (vdir) source
)

// sourceGlyph returns the provenance glyph for a source type.
func sourceGlyph(sourceType string) string {
	if sourceType == "dav" {
		return IconCloud
	}
	return IconDisk
}

// Theme holds the styles for the whole app. A single accent color drives focus.
type Theme struct {
	Accent    lipgloss.Color
	Subtle    lipgloss.Color
	Muted     lipgloss.Color
	Border    lipgloss.Color
	FocusBord lipgloss.Color

	Title      lipgloss.Style
	StatusBar  lipgloss.Style
	StatusKey  lipgloss.Style
	StatusMode lipgloss.Style

	boxStyle lipgloss.Style
	boxFocus lipgloss.Style
	ColTitle lipgloss.Style

	Item         lipgloss.Style
	ItemSelected lipgloss.Style
	ItemDim      lipgloss.Style

	Label lipgloss.Style
	Value lipgloss.Style

	DayHeader lipgloss.Style
	TimeCol   lipgloss.Style
	Today     lipgloss.Style
	Help      lipgloss.Style
}

// DefaultTheme returns Yoro's default styling.
func DefaultTheme() Theme {
	accent := lipgloss.Color("#7aa2f7")
	subtle := lipgloss.Color("#9aa5ce")
	muted := lipgloss.Color("#565f89")
	border := lipgloss.Color("#3b4261")
	focus := accent

	t := Theme{
		Accent:    accent,
		Subtle:    subtle,
		Muted:     muted,
		Border:    border,
		FocusBord: focus,
	}

	t.Title = lipgloss.NewStyle().Bold(true).Foreground(accent)
	t.StatusBar = lipgloss.NewStyle().Foreground(subtle)
	t.StatusKey = lipgloss.NewStyle().Foreground(accent).Bold(true)
	t.StatusMode = lipgloss.NewStyle().Foreground(lipgloss.Color("#1a1b26")).
		Background(accent).Bold(true).Padding(0, 1)

	t.boxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(border)
	t.boxFocus = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(focus)
	t.ColTitle = lipgloss.NewStyle().Bold(true).Foreground(subtle)

	t.Item = lipgloss.NewStyle().Foreground(lipgloss.Color("#c0caf5"))
	t.ItemSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("#1a1b26")).
		Background(accent).Bold(true)
	t.ItemDim = lipgloss.NewStyle().Foreground(muted)

	t.Label = lipgloss.NewStyle().Foreground(muted)
	t.Value = lipgloss.NewStyle().Foreground(lipgloss.Color("#c0caf5"))

	t.DayHeader = lipgloss.NewStyle().Bold(true).Foreground(accent)
	t.TimeCol = lipgloss.NewStyle().Foreground(subtle)
	t.Today = lipgloss.NewStyle().Foreground(lipgloss.Color("#1a1b26")).
		Background(accent).Bold(true)
	t.Help = lipgloss.NewStyle().Foreground(subtle)

	return t
}

// SelectStyle returns the style for a row given whether it is the cursor row and
// whether its column is focused.
func (t Theme) SelectStyle(selected, focused bool) lipgloss.Style {
	switch {
	case selected && focused:
		return t.ItemSelected
	case selected:
		return t.Item.Bold(true).Background(lipgloss.Color("#292e42"))
	default:
		return t.Item
	}
}
