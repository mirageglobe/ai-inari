// agents_data.go owns the Agents view's data and lifecycle handlers: theme
// changes, terminal resize, spinner ticks, and the session/model-caps/running
// fetch results that populate the table. it does NOT own mutation results
// (agents_mutations.go), input (agents_input.go), or the Update dispatch (agents.go).

package views

import (
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

func (h Agents) onThemeChanged() (tea.Model, tea.Cmd) {
	ApplyTableStyles(&h.table)
	h.spinner.Style = spinnerStyle
	return h, nil
}

func (h Agents) onThemeSaveErr(msg ThemeSaveErrMsg) (tea.Model, tea.Cmd) {
	h.status = connErrStyle.Render("theme save failed: " + msg.Err.Error())
	return h, nil
}

func (h Agents) onWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	h.width = msg.Width
	h.height = msg.Height
	// pre-render the hint at the actual width to count its line height.
	// on narrow terminals (~80 chars) the hint wraps to 2 lines; using a fixed
	// reservation of 1 would cause a 1-line overflow that scrolls the alt screen
	// and pushes the root header off the top of the display.
	hintStr := RenderHint(agentsHints(false, h.offline), h.width)
	h.hintHeight = strings.Count(hintStr, "\n") + 1
	// topbar(1) + border-top(1) + col-header(1) + border-bottom(1) + sessionLine(1) + cwdLine(1) + statusLine(1) + input(1) + hint(hintHeight)
	tableHeight := msg.Height - 8 - h.hintHeight
	if tableHeight < 1 {
		tableHeight = 1
	}
	h.tableHeight = tableHeight
	h.table.SetHeight(tableHeight)
	// resize model column to fill the shared modal width (capped, not raw terminal
	// width, so wide terminals do not stretch the popup past the UIWidth budget).
	// fixed cols: indicator(2) + name(20) + vram(12) + status(16) + context(12) = 62
	// plus 6 cols x 2 cell padding = 12; model takes the rest of the inner width.
	modelColW := modalInnerWidth(h.width) - 74
	if modelColW < 10 {
		modelColW = 10
	}
	h.table.SetColumns([]table.Column{
		{Title: "", Width: 2},
		{Title: "name", Width: 20},
		{Title: "model", Width: modelColW},
		{Title: "vram", Width: 12},
		{Title: "status", Width: 16},
		{Title: "context", Width: 12},
	})
	return h, nil
}

func (h Agents) onTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if h.loading {
		var cmd tea.Cmd
		h.spinner, cmd = h.spinner.Update(msg)
		return h, cmd
	}
	return h, nil
}

func (h Agents) onSessions(msg sessionsMsg) (tea.Model, tea.Cmd) {
	h.loading = false
	if msg.err != nil {
		log.Printf("session fetch error: %v", msg.err)
	} else {
		h.status = ""
		h.sessions = msg.sessions
		if len(msg.sessions) == 0 && !h.autoCreated {
			h.autoCreated = true
			return h, createSessionCmd(h.client, "default agent")
		}
		// on first successful load, auto-open the first session that has a model.
		if h.autoOpen && len(msg.sessions) > 0 {
			h.autoOpen = false
			for _, s := range msg.sessions {
				if s.Model != "" {
					sess := s
					return h, func() tea.Msg {
						return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars, SystemPrompt: sess.SystemPrompt}
					}
				}
			}
			h.autoOpen = false
		}
	}
	h.rebuildTable()
	// fetch caps for any model not yet cached
	var cmds []tea.Cmd
	for _, s := range h.sessions {
		if s.Model != "" {
			if _, ok := h.modelCaps[s.Model]; !ok {
				cmds = append(cmds, fetchModelCapsCmd(h.client, s.Model))
			}
		}
	}
	return h, tea.Batch(cmds...)
}

func (h Agents) onModelCaps(msg modelCapsMsg) (tea.Model, tea.Cmd) {
	if h.modelCaps == nil {
		h.modelCaps = make(map[string][]string)
	}
	h.modelCaps[msg.model] = msg.caps
	h.rebuildTable()
	return h, nil
}

func (h Agents) onRunning(msg runningMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Printf("running fetch error: %v", msg.err)
	}
	// refresh live stats for display only; sessions are user-created, not derived from running models.
	h.runningInfo = make(map[string]runningMeta, len(msg.models))
	for _, m := range msg.models {
		h.runningInfo[m.Name] = runningMeta{vram: m.SizeVRAM, expiry: m.ExpiresAt}
	}
	h.rebuildTable()
	return h, nil
}
