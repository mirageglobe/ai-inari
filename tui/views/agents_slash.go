package views

import (
	tea "github.com/charmbracelet/bubbletea"
)

// agentsCommands is the ordered list of valid slash commands used for autocomplete suggestions.
// agent actions (add/chat/delete/export/logs/describe) are hotkeys instead, see agentsHints.
// /help, /quit, /theme, /refresh, and /chat live in the chat view now that chat is the main view.
var agentsCommands = []string{
	"/model",
	"/model unload",
}

func (h Agents) handleSlashCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/model":
		if h.offline {
			h.infoMsg = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		idx := h.table.Cursor()
		if idx >= 0 && idx < len(h.sessions) {
			sess := h.sessions[idx]
			return h, func() tea.Msg {
				return OpenModelSelectorMsg{SessionID: sess.ID, SessionName: sess.Name}
			}
		}
	case "/model unload":
		if h.offline {
			h.infoMsg = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		idx := h.table.Cursor()
		if idx >= 0 && idx < len(h.sessions) {
			sess := h.sessions[idx]
			if sess.Model != "" {
				h.sessions[idx].Model = ""
				h.rebuildTable()
				return h, unassignModelCmd(h.client, sess.ID, sess.Name, sess.Model)
			}
		}
	default:
		h.infoMsg = modelsStyle.Render("[warn] unknown command: " + cmd)
	}
	return h, nil
}
