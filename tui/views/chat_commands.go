// chat_commands.go owns the chat view's slash-command vocabulary: the canonical
// command table, its contextual enable/disable predicates, the palette
// suggestion builder, and the derived help-overlay rows.
// it is the single source of truth for the command set; the palette
// (renderChatSuggestions) and the help overlay both derive from chatCommandTable.
// it does NOT own command dispatch; that stays in handleSlashCommand (chat.go).

package views

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ChatCommand describes one slash command in the chat view. Enabled reports
// whether the command is currently actionable given chat state; a nil Enabled
// means the command is always available.
type ChatCommand struct {
	Name    string
	Desc    string
	Enabled func(c Chat) bool
}

// chatCommandTable is the canonical set of chat slash commands, sorted
// alphabetically by name in init() so the palette and help overlay both list
// commands in a predictable order regardless of literal ordering here.
// the three context-gated commands mirror the validity guards already enforced
// in handleSlashCommand, so the palette dims exactly what dispatch would reject.
var chatCommandTable = []ChatCommand{
	{"/clear", "clear message context", nil},
	{"/compact", "summarise and compress context", nil},
	{"/copy", "copy last response to clipboard", func(c Chat) bool { return c.lastAssistantText() != "" }},
	{"/export", "save full context to a file", nil},
	{"/model", "assign/unload model (modal; [u] to unload)", nil},
	{"/describe", "open session metadata/config view", nil},
	{"/logs", "open logs view", nil},
	{"/cwd", "change working directory (/cwd <path>)", nil},
	{"/rename", "rename this session (/rename <name>)", nil},
	{"/tools", "toggle builtin tools panel", func(c Chat) bool { return c.cwd != "" }},
	{"/agents", "open agents as a popup ([q] to close)", nil},
	{"/chat", "jump to default agent's chat", nil},
	{"/refresh", "reload agents session list", nil},
	{"/theme", "cycle theme", nil},
	{"/help", "toggle this help overlay", nil},
	{"/quit", "quit", nil},
}

// cmdSuggestion is a single palette entry: a command name and whether it is
// currently enabled in the active chat context.
type cmdSuggestion struct {
	Name    string
	Enabled bool
}

// matchingCommands returns the table commands whose name starts with prefix,
// each tagged with its current enabled state. pure and order-preserving.
func (c Chat) matchingCommands(prefix string) []cmdSuggestion {
	var out []cmdSuggestion
	for _, cmd := range chatCommandTable {
		if !strings.HasPrefix(cmd.Name, prefix) {
			continue
		}
		enabled := cmd.Enabled == nil || cmd.Enabled(c)
		out = append(out, cmdSuggestion{Name: cmd.Name, Enabled: enabled})
	}
	return out
}

// renderChatSuggestions replaces the hint bar while the user is typing a slash
// command, showing matching commands and dimming those not currently actionable.
func (c Chat) renderChatSuggestions(prefix string, width int) string {
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	_ = width

	matches := c.matchingCommands(prefix)
	label := labelStyle.Render("[cmd]") + " "
	if len(matches) == 0 {
		return label + dimStyle.Render("no match")
	}

	const gap = "  "
	parts := make([]string, len(matches))
	for i, m := range matches {
		if m.Enabled {
			parts[i] = activeStyle.Render(m.Name)
		} else {
			parts[i] = dimStyle.Render(m.Name)
		}
	}
	return label + strings.Join(parts, gap)
}

// buildChatHelp assembles the chat view's help-overlay rows: the fixed keys
// that bracket the command list, with the slash commands derived from
// chatCommandTable so help can never drift from the actual command set.
func buildChatHelp() []helpEntry {
	entries := []helpEntry{
		{"[enter]", "send message"},
		{"[ctrl+p]", "open slash command palette"},
	}
	for _, cmd := range chatCommandTable {
		entries = append(entries, helpEntry{"[" + cmd.Name + "]", cmd.Desc})
	}
	entries = append(entries,
		helpEntry{"[↑] / [↓]", "navigate input history"},
		helpEntry{"[esc]", "exit tools panel / clear slash input"},
	)
	return entries
}

// sort the command table alphabetically, then register the derived chat help
// rows so helpByView["chat"] stays in sync with the command table rather than
// being a hand-maintained parallel list. sorting here keeps both the palette and
// the help overlay alphabetical without hand-ordering the literal.
func init() {
	sort.Slice(chatCommandTable, func(i, j int) bool {
		return chatCommandTable[i].Name < chatCommandTable[j].Name
	})
	helpByView["chat"] = buildChatHelp()
}
