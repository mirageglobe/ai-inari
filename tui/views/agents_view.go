package views

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

// agentsHints returns the command hint list for the agents view.
// the view is hotkey-only: model selection, export, logs, and describe all live in
// chat, so this list is just the session-table actions.
func agentsHints(hasSession, offline bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	return []HintCmd{
		hc("[a] add", !offline),
		hc("[enter] chat", hasSession && !offline),
		hc("[x] delete", hasSession && !offline),
	}
}

// RenderModal renders the agents popup as a centred overlay, matching the
// model-selector modal's shape: a title, the table, and the hint line inside
// a single rounded-border box (see ModelSelector.RenderModal in selector.go).
func (h Agents) RenderModal(termWidth, termHeight int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary)
	title := titleStyle.Render("agents")

	idx := h.table.Cursor()
	hasSession := idx >= 0 && idx < len(h.sessions)

	hints := agentsHints(hasSession, h.offline)
	hints = append(hints, HS(), H("[q/esc] back to chat"))
	hint := RenderHint(hints, modalInnerWidth(h.width))

	var lines []string
	lines = append(lines, title)
	if h.loading {
		pad := lipgloss.NewStyle().PaddingTop(1).PaddingLeft(1)
		lines = append(lines, pad.Render(h.spinner.View()+" fetching agents..."))
	} else {
		lines = append(lines, h.table.View())
	}
	lines = append(lines, hint)

	boxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ActiveTheme.Primary).
		Padding(0, 1)

	box := boxStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(termWidth, termHeight, lipgloss.Center, lipgloss.Center, box)
}

func (h Agents) View() string {
	idx := h.table.Cursor()
	hasSession := idx >= 0 && idx < len(h.sessions)

	sessionName := "agent"
	if hasSession {
		sessionName = h.sessions[idx].Name
	}

	model := "-"
	tokens := "-"
	cwd := ""
	if hasSession {
		sess := h.sessions[idx]
		if sess.Model != "" {
			model = sess.Model
		}
		if sess.ContextChars > 0 {
			tokens = fmtTokens(sess.ContextChars)
		}
		if sess.CWD != "" {
			cwd = sess.CWD
		}
	}

	sessionLine := RenderSessionLine("agents", sessionName, model, tokens)
	cwdLine := renderCWDLine(cwd)

	var statusContent string
	switch {
	case h.status != "" && h.infoMsg != "":
		statusContent = h.status + "  " + h.infoMsg
	case h.status != "":
		statusContent = h.status
	case h.infoMsg != "":
		statusContent = h.infoMsg
	}
	statusLine := renderStatusLine(statusContent)

	hintLine := RenderHint(agentsHints(hasSession, h.offline), h.width)

	if h.loading {
		pad := lipgloss.NewStyle().PaddingTop(4).PaddingLeft(2)
		body := agentsStyle.Render(pad.Render(h.spinner.View() + " fetching agents..."))
		return body + "\n" + renderFooter(sessionLine, cwdLine, statusLine, "", hintLine)
	}

	body := agentsStyle.Render(h.table.View())
	return body + "\n" + renderFooter(sessionLine, cwdLine, statusLine, "", hintLine)
}

func (h *Agents) rebuildTable() {
	sort.Slice(h.sessions, func(i, j int) bool {
		return h.sessions[i].Name < h.sessions[j].Name
	})
	rows := make([]table.Row, len(h.sessions))
	for i, s := range h.sessions {
		vram, status := "-", "-"
		if info, ok := h.runningInfo[s.Model]; ok {
			vram = formatBytes(info.vram)
			status = formatExpiry(info.expiry)
		} else if s.Model != "" {
			// model assigned but not currently loaded in ollama memory
			status = "sleeping"
		}
		model := s.Model
		if model == "" {
			model = "-"
		} else if caps, ok := h.modelCaps[s.Model]; ok {
			for _, c := range caps {
				switch c {
				case "tools":
					model += " [tool]"
				case "vision":
					model += " [vis]"
				}
			}
		}
		ctx := "-"
		if s.ContextChars > 0 {
			ctx = fmtTokens(s.ContextChars)
		}
		indicator := " "
		if s.ID == h.activeSessionID {
			indicator = ">"
		}
		rows[i] = table.Row{indicator, s.Name, model, vram, status, ctx}
	}
	h.table.SetRows(rows)
}

// SelectedSession returns the session at the current cursor plus its vram.
// returns false if no session is under the cursor.
func (h Agents) SelectedSession() (ipc.SessionInfo, int64, bool) {
	idx := h.table.Cursor()
	if idx < 0 || idx >= len(h.sessions) {
		return ipc.SessionInfo{}, 0, false
	}
	sess := h.sessions[idx]
	return sess, h.runningInfo[sess.Model].vram, true
}

// DefaultSession returns the first session in the list (by name, since the
// table is sorted alphabetically), used by chat's /chat command to jump back
// to the default agent regardless of which session is currently active.
func (h Agents) DefaultSession() (ipc.SessionInfo, bool) {
	if len(h.sessions) == 0 {
		return ipc.SessionInfo{}, false
	}
	return h.sessions[0], true
}

func (h Agents) usedNames() []string {
	names := make([]string, len(h.sessions))
	for i, s := range h.sessions {
		names[i] = s.Name
	}
	return names
}
