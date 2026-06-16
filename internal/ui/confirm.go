package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// confirmPrompt is the modal overlay for confirming a destructive action
// (currently delete). It captures keys until the user answers y or n/esc; App
// reads target on confirm and performs the deletion via the store.
type confirmPrompt struct {
	theme   Theme
	title   string
	message string

	domain Mode   // which pane the item belongs to
	colID  string // owning collection
	path   string // write-back locator of the item to delete
}

// view renders the prompt centered in the given terminal size.
func (c *confirmPrompt) view(width, height int) string {
	yes := c.theme.StatusKey.Render(" y ") + c.theme.Value.Render(" delete")
	no := c.theme.StatusKey.Render(" n ") + c.theme.Value.Render(" cancel")

	var b strings.Builder
	b.WriteString(c.theme.Title.Render(c.title) + "\n\n")
	b.WriteString(c.theme.Value.Render(c.message) + "\n")
	b.WriteString("\n" + yes + "    " + no)
	b.WriteString("\n" + c.theme.Help.Render("esc also cancels"))

	content := b.String()
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		c.theme.Column("", content, 50, lipgloss.Height(content)+2, true))
}
