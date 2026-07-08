package views

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

// agentsHints returns the default command hint list for the agents view.
// hasSession, hasModel, and offline control which items are enabled.
func agentsHints(hasSession, _ /* hasModel */, offline bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	return []HintCmd{
		hc("/chat", !offline),
		hc("/agent", !offline),
		hc("/model", hasSession && !offline),
		HS(),
		hc("/refresh", !offline),
		H("/theme"),
		H("/help"),
		H("/quit"),
	}
}

// defaultHints returns the expanded /default sub-command hint list.
func defaultHints(hasDefault, offline bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	return []HintCmd{
		hc("/chat", hasDefault && !offline),
	}
}

// modelHints returns the expanded /model sub-command hint list shown when the user is typing /model.
func modelHints(hasSession, hasModel, offline bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	return []HintCmd{
		hc("/model select", hasSession && !offline),
		hc("/model unload", hasModel && !offline),
	}
}

// agentHints returns the expanded /agent sub-command hint list shown when the user is typing /agent.
func agentHints(hasSession, hasModel, offline bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	return []HintCmd{
		hc("/agent add", !offline),
		hc("/agent chat", hasModel && !offline),
		hc("/agent describe", hasSession && !offline),
		hc("/agent export", hasSession),
		hc("/agent logs", !offline),
		hc("/agent delete", hasSession && !offline),
	}
}

func (h Agents) View() string {
	idx := h.table.Cursor()
	hasSession := idx >= 0 && idx < len(h.sessions)
	hasModel := hasSession && h.sessions[idx].Model != ""

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

	hasDefault := len(h.sessions) > 0 && h.sessions[0].Model != ""
	var hints []HintCmd
	switch {
	case strings.HasPrefix(h.input.Value(), "/chat"):
		hints = defaultHints(hasDefault, h.offline)
	case strings.HasPrefix(h.input.Value(), "/agent"):
		hints = agentHints(hasSession, hasModel, h.offline)
	case strings.HasPrefix(h.input.Value(), "/model"):
		hints = modelHints(hasSession, hasModel, h.offline)
	default:
		hints = agentsHints(hasSession, hasModel, h.offline)
	}
	if h.modal {
		hints = append([]HintCmd{H("[q] back to chat")}, hints...)
	}
	hintLine := RenderHint(hints, h.width)

	if h.loading {
		pad := lipgloss.NewStyle().PaddingTop(4).PaddingLeft(2)
		body := agentsStyle.Render(pad.Render(h.spinner.View() + " fetching agents..."))
		return body + "\n" + renderFooter(sessionLine, cwdLine, statusLine, "", hintLine)
	}

	body := agentsStyle.Render(h.table.View())
	return body + "\n" + renderFooter(sessionLine, cwdLine, statusLine, h.input.View(), hintLine)
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

func (h Agents) usedNames() []string {
	names := make([]string, len(h.sessions))
	for i, s := range h.sessions {
		names[i] = s.Name
	}
	return names
}
