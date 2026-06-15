package ui

import (
	"encoding/base64"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// copyToClipboard returns a command that copies s to the system clipboard using
// the OSC52 terminal escape, wrapped for tmux passthrough when running inside
// tmux. This works over SSH and does not require an external clipboard tool.
func copyToClipboard(s string) tea.Cmd {
	return func() tea.Msg {
		enc := base64.StdEncoding.EncodeToString([]byte(s))
		seq := "\x1b]52;c;" + enc + "\x07"
		if os.Getenv("TMUX") != "" {
			// tmux passthrough: wrap, doubling every inner ESC.
			seq = "\x1bPtmux;" + osc52DoubleEsc(seq) + "\x1b\\"
		}
		_, _ = os.Stdout.WriteString(seq)
		return nil
	}
}

func osc52DoubleEsc(s string) string {
	out := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			out = append(out, 0x1b, 0x1b)
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}
