package ui

import "github.com/charmbracelet/lipgloss"

// yoroBanner is the ASCII wordmark shown on the loading splash. Pure text so it
// renders in every terminal; styling is applied at draw time from the theme.
const yoroBanner = `__   __ ___  ____   ___
\ \ / // _ \|  _ \ / _ \
 \ V /| | | | |_) | | | |
  | | | |_| |  _ <| |_| |
  |_|  \___/|_| \_\\___/`

const bannerTagline = "calendars & contacts"

// splash centers the wordmark, tagline, and a body line (spinner/error) for the
// loading and load-error screens.
func (a App) splash(body string) string {
	logo := a.theme.Title.Render(yoroBanner)
	tag := lipgloss.NewStyle().Foreground(a.theme.Muted).Render(bannerTagline)
	content := lipgloss.JoinVertical(lipgloss.Center, logo, tag, "", body)
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
}
