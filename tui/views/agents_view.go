package views

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// agentsHints returns the command hint list for the agents view. while the
// filter input is focused the hints switch to filter-editing keys; otherwise
// they are the session-table actions plus the [/] filter entry.
func agentsHints(hasSession, offline, filtering bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	if filtering {
		return []HintCmd{
			hc("type to filter", true),
			hc("[enter] apply", true),
			hc("[esc] clear", true),
		}
	}
	return []HintCmd{
		hc("[a] add", !offline),
		hc("[enter] chat", hasSession && !offline),
		hc("[x] delete", hasSession && !offline),
		hc("[/] filter", true),
	}
}

// renderFilterLine renders the footer filter row: the current query (with a
// block cursor while focused) and an "N of M" count. returns "" when no filter
// is active and the input is not focused, so the footer slot stays blank.
func renderFilterLine(filter string, filtering bool, shown, total int) string {
	if !filtering && filter == "" {
		return ""
	}
	labelStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
	txt := filter
	if filtering {
		txt += "█" // block cursor while focused; avoids ANSI cursor-shape juggling
	}
	return labelStyle.Render("[filter]") + " " + txt + "  " + labelStyle.Render(fmt.Sprintf("(%d of %d)", shown, total))
}

// RenderModal renders the agents popup as a centred overlay, matching the
// model-selector modal's shape: a title, the table, and the hint line inside
// a single rounded-border box (see ModelSelector.RenderModal in selector.go).
func (h Agents) RenderModal(termWidth, termHeight int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary)
	title := titleStyle.Render("agents")

	idx := h.table.Cursor()
	hasSession := idx >= 0 && idx < len(h.sessions)

	hints := agentsHints(hasSession, h.offline, h.filtering)
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
	if fl := renderFilterLine(h.filter, h.filtering, len(h.sessions), len(h.allSessions)); fl != "" {
		lines = append(lines, fl)
	}
	lines = append(lines, hint)

	return renderModalBox(lines, termWidth, termHeight)
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

	hintLine := RenderHint(agentsHints(hasSession, h.offline, h.filtering), h.width)
	filterLine := renderFilterLine(h.filter, h.filtering, len(h.sessions), len(h.allSessions))

	if h.loading {
		pad := lipgloss.NewStyle().PaddingTop(4).PaddingLeft(2)
		body := agentsStyle.Render(pad.Render(h.spinner.View() + " fetching agents..."))
		return body + "\n" + renderFooter(sessionLine, cwdLine, statusLine, filterLine, hintLine)
	}

	body := agentsStyle.Render(h.table.View())
	return body + "\n" + renderFooter(sessionLine, cwdLine, statusLine, filterLine, hintLine)
}
