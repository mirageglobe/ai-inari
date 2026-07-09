// help overlay for the inari TUI.
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
	"agents": {
		{"[a]", "create session"},
		{"[enter]", "open chat"},
		{"[x]", "delete session"},
		{"[q] / [esc]", "back to chat (popup only)"},
	},
	"chat": {
		{"[enter]", "send message"},
		{"[ctrl+p]", "open slash command palette"},
		{"[/clear]", "clear message context"},
		{"[/compact]", "summarise and compress context"},
		{"[/copy]", "copy last response to clipboard"},
		{"[/export]", "save full context to a file"},
		{"[/model]", "assign model (modal)"},
		{"[/model unload]", "unload model"},
		{"[/describe]", "open session metadata/config view"},
		{"[/logs]", "open logs view"},
		{"[/tools]", "toggle builtin tools panel"},
		{"[/agents]", "open agents as a popup ([q] to close)"},
		{"[/chat]", "jump to default agent's chat"},
		{"[/refresh]", "reload agents session list"},
		{"[/theme]", "cycle theme"},
		{"[/help]", "toggle this help overlay"},
		{"[/quit]", "quit"},
		{"[↑] / [↓]", "navigate input history"},
		{"[esc]", "exit tools panel / clear slash input"},
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
// viewName must match a key in helpByView (e.g. "agents", "chat").
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
