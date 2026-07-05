package views

import (
	tea "github.com/charmbracelet/bubbletea"
)

// agentsCommands is the ordered list of valid slash commands used for autocomplete suggestions.
var agentsCommands = []string{
	"/chat",
	"/agent add",
	"/model select",
	"/model unload",
	"/agent chat",
	"/agent delete",
	"/agent export",
	"/refresh",
	"/agent logs",
	"/agent describe",
	"/theme",
	"/help",
	"/quit",
}

func (h Agents) handleSlashCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/agent add":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		name := pickFoxName(h.usedNames())
		return h, createSessionCmd(h.client, name)
	case "/model select":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
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
			h.foxInfo = modelsStyle.Render("[warn] offline")
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
	case "/chat":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		// open chat for the first session in the list regardless of cursor position.
		if len(h.sessions) > 0 {
			sess := h.sessions[0]
			if sess.Model != "" {
				return h, func() tea.Msg {
					return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars}
				}
			}
			h.foxInfo = modelsStyle.Render("[warn] default kitsune has no model assigned")
		}
	case "/agent chat":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		idx := h.table.Cursor()
		if idx >= 0 && idx < len(h.sessions) {
			sess := h.sessions[idx]
			if sess.Model != "" {
				return h, func() tea.Msg {
					return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars}
				}
			}
		}
	case "/agent delete":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		idx := h.table.Cursor()
		if idx >= 0 && idx < len(h.sessions) {
			id := h.sessions[idx].ID
			return h, deleteSessionCmd(h.client, id)
		}
	case "/agent export":
		idx := h.table.Cursor()
		if idx >= 0 && idx < len(h.sessions) {
			sess := h.sessions[idx]
			return h, exportChatCmd(h.client, sess.ID, sess.Name)
		}
	case "/refresh":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		h.status = ""
		h.loading = true
		return h, tea.Batch(fetchSessions(h.client), fetchRunning(h.client), h.spinner.Tick)
	case "/agent logs":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		return h, func() tea.Msg { return OpenLogsMsg{} }
	case "/agent describe":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		return h, func() tea.Msg { return OpenDescribeMsg{} }
	case "/theme":
		return h, func() tea.Msg { return CycleThemeMsg{} }
	case "/help":
		return h, func() tea.Msg { return ToggleHelpMsg{} }
	case "/quit":
		return h, tea.Quit
	default:
		h.foxInfo = modelsStyle.Render("[warn] unknown command: " + cmd)
	}
	return h, nil
}
