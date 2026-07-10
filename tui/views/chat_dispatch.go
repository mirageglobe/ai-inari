// chat_dispatch.go owns handleSlashCommand: the chat view's mapping from a
// slash command string to the model update and command it triggers.
// it does NOT own the command vocabulary (names, descriptions, enabled state);
// that is chat_commands.go. the two are kept in sync by a test.

package views

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleSlashCommand dispatches a slash command to its handler. an unrecognised
// command falls through to the default case and surfaces a warning; the command
// palette (chat_commands.go) only ever offers names present in chatCommandTable.
func (c Chat) handleSlashCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/clear":
		id := c.sessionID
		return c, func() tea.Msg {
			return clearHistoryResultMsg{err: c.client.ClearHistory(id)}
		}
	case "/compact":
		id := c.sessionID
		c.waiting = true
		c.status = "compacting…"
		return c, tea.Batch(c.spinner.Tick, func() tea.Msg {
			summary, err := c.client.CompactHistory(id)
			return compactHistoryResultMsg{summary: summary, err: err}
		})
	case "/copy":
		// copy the most recent assistant response to the system clipboard.
		text := c.lastAssistantText()
		if text == "" {
			c.status = "[warn] no response to copy"
			return c, nil
		}
		if err := copyToClipboard(text); err != nil {
			c.status = "[warn] copy failed: " + err.Error()
		} else {
			c.status = "[copied] response"
		}
		return c, nil
	case "/export":
		// download the full session context (history) to a text file; reuses the
		// agents export path so both entry points write to the same location.
		return c, exportChatCmd(c.client, c.sessionID, c.sessionName)
	case "/model":
		return c, func() tea.Msg {
			return OpenModelSelectorMsg{SessionID: c.sessionID, SessionName: c.sessionName}
		}
	case "/model unload":
		if c.model == "" {
			c.status = "[warn] no model assigned"
			return c, nil
		}
		return c, unassignModelCmd(c.client, c.sessionID, c.sessionName, c.model)
	case "/tools":
		if c.cwd != "" {
			c.showBuiltin = !c.showBuiltin
		} else {
			c.status = "[warn] tools not available (no cwd set)"
		}
		return c, nil
	case "/help":
		return c, func() tea.Msg { return ToggleHelpMsg{} }
	case "/describe":
		return c, func() tea.Msg { return OpenDescribeMsg{} }
	case "/logs":
		return c, func() tea.Msg { return OpenLogsMsg{} }
	case "/agents":
		return c, func() tea.Msg { return OpenAgentsMsg{} }
	case "/chat":
		return c, func() tea.Msg { return OpenDefaultChatMsg{} }
	case "/refresh":
		return c, func() tea.Msg { return RefreshAgentsMsg{} }
	case "/theme":
		return c, func() tea.Msg { return CycleThemeMsg{} }
	case "/quit":
		return c, tea.Quit
	default:
		c.status = "[warn] unknown command: " + cmd
		return c, nil
	}
}
