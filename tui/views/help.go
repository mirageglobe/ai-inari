// Package views — help overlay for the kitsune TUI.
// this file owns the per-view key reference shown when [?] is pressed.
// it does NOT own key dispatch — the root model intercepts [?] and [esc].
package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type helpEntry struct {
	key  string
	desc string
}

// helpByView maps view name strings to their ordered key reference entries.
// update this table whenever a new binding is added to a view.
var helpByView = map[string][]helpEntry{
	"herd": {
		{"[s]", "new session"},
		{"[m]", "assign model"},
		{"[c] / [enter]", "open chat"},
		{"[u]", "unload model"},
		{"[x]", "delete session"},
		{"[r]", "refresh"},
		{"[l]", "logs"},
		{"[d]", "describe"},
		{"[q]", "quit"},
	},
	"chat": {
		{"[enter]", "send message"},
		{"[ctrl+o]", "change model"},
		{"[ctrl+f]", "toggle tools panel"},
		{"[↑] / [↓]", "scroll history"},
		{"[esc]", "back to sessions"},
	},
	"describe": {
		{"[e]", "edit system prompt"},
		{"[ctrl+s]", "save (in edit mode)"},
		{"[esc]", "cancel / back"},
	},
	"logs": {
		{"[r]", "refresh log"},
		{"[esc]", "back"},
	},
	"models": {
		{"[enter] / [l]", "assign model to session"},
		{"[↑] / [↓]", "navigate list"},
		{"[esc]", "back"},
	},
}

var (
	helpTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	helpKeyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	helpDescStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	helpFootStyle  = lipgloss.NewStyle().Faint(true)
	helpBoxStyle   = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(1, 3)
)

// RenderHelpOverlay returns a help modal centered in a termWidth × termHeight area.
// viewName must match a key in helpByView (e.g. "herd", "chat").
// termHeight should exclude the top bar row so the modal sits in the body area.
func RenderHelpOverlay(viewName string, termWidth, termHeight int) string {
	entries := helpByView[viewName]

	var sb strings.Builder
	sb.WriteString(helpTitleStyle.Render("help — " + viewName))
	sb.WriteString("\n\n")

	for _, e := range entries {
		fmt.Fprintf(&sb, "%s  %s\n",
			helpKeyStyle.Render(fmt.Sprintf("%-18s", e.key)),
			helpDescStyle.Render(e.desc),
		)
	}

	sb.WriteString("\n")
	sb.WriteString(helpFootStyle.Render("[?] or [esc] to close"))

	box := helpBoxStyle.Render(sb.String())
	return lipgloss.Place(termWidth, termHeight, lipgloss.Center, lipgloss.Center, box)
}
