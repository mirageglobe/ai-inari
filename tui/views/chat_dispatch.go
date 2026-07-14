// chat_dispatch.go owns handleSlashCommand: the chat view's mapping from a
// slash command string to the model update and command it triggers.
// it does NOT own the command vocabulary (names, descriptions, enabled state);
// that is chat_commands.go. the two are kept in sync by a test.

package views

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleSlashCommand dispatches a slash command to its handler. an unrecognised
// command falls through to the default case and surfaces a warning; the command
// palette (chat_commands.go) only ever offers names present in chatCommandTable.
func (c Chat) handleSlashCommand(cmd string) (tea.Model, tea.Cmd) {
	// /cwd takes a path argument, so match it by prefix before the exact switch.
	if cmd == "/cwd" || strings.HasPrefix(cmd, "/cwd ") {
		path := strings.TrimSpace(strings.TrimPrefix(cmd, "/cwd"))
		if path == "" {
			c.status = "[warn] usage: /cwd <path>"
			return c, nil
		}
		id := c.sessionID
		return c, func() tea.Msg {
			info, err := c.client.SetCwd(id, path)
			return setCwdResultMsg{info: info, err: err}
		}
	}
	// /rename takes a name argument, so match it by prefix before the exact switch.
	if cmd == "/rename" || strings.HasPrefix(cmd, "/rename ") {
		name := strings.TrimSpace(strings.TrimPrefix(cmd, "/rename"))
		if name == "" {
			c.status = "[warn] usage: /rename <name>"
			return c, nil
		}
		id := c.sessionID
		return c, func() tea.Msg {
			info, err := c.client.Rename(id, name)
			return renameResultMsg{info: info, err: err}
		}
	}
	// /tag takes a label argument, so match it by prefix before the exact switch.
	if cmd == "/tag" || strings.HasPrefix(cmd, "/tag ") {
		tag := strings.TrimSpace(strings.TrimPrefix(cmd, "/tag"))
		if tag == "" {
			c.status = "[warn] usage: /tag <label>"
			return c, nil
		}
		id := c.sessionID
		return c, func() tea.Msg {
			info, err := c.client.Tag(id, tag)
			return tagResultMsg{info: info, err: err}
		}
	}
	// /numctx [n]: no arg shows the current window; a number sets the override; 0
	// or "auto" clears it (revert to the model-derived default).
	if cmd == "/numctx" || strings.HasPrefix(cmd, "/numctx ") {
		arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/numctx"))
		if arg == "" {
			c.status = "[info] ctx window: " + strconv.Itoa(effectiveNumCtx(c.numCtxOverride, c.maxCtx))
			return c, nil
		}
		n := 0
		if arg != "auto" {
			parsed, err := strconv.Atoi(arg)
			if err != nil || parsed < 0 {
				c.status = "[warn] usage: /numctx <tokens> | 0 | auto"
				return c, nil
			}
			n = parsed
		}
		id := c.sessionID
		return c, func() tea.Msg {
			info, err := c.client.SetNumCtx(id, n)
			return setNumCtxResultMsg{info: info, err: err}
		}
	}
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
			return OpenModelSelectorMsg{SessionID: c.sessionID, SessionName: c.sessionName, Model: c.model}
		}
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
