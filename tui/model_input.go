// model_input.go owns the root model's late-stage routing: modal capture (model
// selector / agents popup / theme-save error), keyboard handling (theme picker,
// help overlay, per-view keys), and the final fall-through to the active view.
// it does NOT own broadcast/system messages (model_router.go), navigation
// (model_nav.go), or the Update dispatch (model.go).

package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/tui/views"
)

// updateModal captures all remaining messages while a modal is active, and
// routes theme-save failures to the active view's status bar.
func (m Model) updateModal(msg tea.Msg) (Model, tea.Cmd, bool) {
	// route all remaining messages to the model selector when the modal is active.
	if m.showModelSelector {
		if key, ok := msg.(tea.KeyMsg); ok && (key.String() == "esc" || key.String() == "q") {
			m.showModelSelector = false
			return m, nil, true
		}
		updated, cmd := m.models.Update(msg)
		m.models = updated.(views.ModelSelector)
		return m, cmd, true
	}
	// route all remaining messages to agents when it is overlaid on chat as a popup.
	if m.showAgents {
		updated, cmd := m.agents.Update(msg)
		m.agents = updated.(views.Agents)
		return m, cmd, true
	}
	// route a theme-save failure to the active view so it shows in the status bar.
	if saveErr, ok := msg.(views.ThemeSaveErrMsg); ok {
		if m.current == viewChat {
			if chat, exists := m.chats[m.activeSession]; exists {
				updated, cmd := chat.Update(saveErr)
				m.chats[m.activeSession] = updated.(views.Chat)
				return m, cmd, true
			}
		}
		updated, cmd := m.agents.Update(saveErr)
		m.agents = updated.(views.Agents)
		return m, cmd, true
	}
	return m, nil, false
}

// updateKeys handles global key bindings: the theme picker, the help overlay,
// and per-view shortcuts. it returns handled false for keys that should fall
// through to the active view (e.g. normal typing in chat).
func (m Model) updateKeys(msg tea.Msg) (Model, tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil, false
	}
	if m.showThemePicker {
		switch key.String() {
		case "up", "k":
			if m.themePickerIdx > 0 {
				m.themePickerIdx--
			}
		case "down", "j":
			if m.themePickerIdx < len(views.Themes)-1 {
				m.themePickerIdx++
			}
		case "enter":
			m.showThemePicker = false
			m.themeIdx = m.themePickerIdx
			mm, cmd := m.applyTheme(m.themeIdx)
			return mm.(Model), cmd, true
		case "esc":
			m.showThemePicker = false
		}
		return m, nil, true
	}

	// [?] toggles help from non-agents, non-chat views; agents uses /help slash command.
	if key.String() == "?" && m.current != viewChat && m.current != viewAgents {
		m.showHelp = !m.showHelp
		return m, nil, true
	}

	// while help is open, only [esc] (or a second [?] in a secondary view) closes it;
	// all other keys are consumed.
	if m.showHelp {
		if key.String() == "esc" {
			m.showHelp = false
		}
		return m, nil, true
	}

	switch m.current {
	case viewChat:
		switch key.String() {
		case "ctrl+o":
			if chat, ok := m.chats[m.activeSession]; ok {
				m.models = m.models.ForSession(chat.SessionID(), chat.SessionName(), chat.Model())
			}
			m.showModelSelector = true
			m.models = m.models.WithModalDimensions()
			return m, m.models.Init(), true
		}
	default:
		// esc from secondary views returns to agents, except when describe is in edit mode
		if key.String() == "esc" && !(m.current == viewDescribe && m.describe.IsEditing()) {
			m.current = viewAgents
			return m, m.agents.Init(), true
		}
	}
	return m, nil, false
}

// updateActiveView forwards a message to whichever view is currently active.
// this is the terminal step of Update, so it always reports the message handled.
func (m Model) updateActiveView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current {
	case viewAgents:
		updated, cmd := m.agents.Update(msg)
		m.agents = updated.(views.Agents)
		return m, cmd
	case viewLogs:
		updated, cmd := m.logs.Update(msg)
		m.logs = updated.(views.Logs)
		return m, cmd
	case viewDescribe:
		updated, cmd := m.describe.Update(msg)
		m.describe = updated.(views.Describe)
		return m, cmd
	case viewChat:
		updated, cmd := m.chats[m.activeSession].Update(msg)
		m.chats[m.activeSession] = updated.(views.Chat)
		return m, cmd
	}
	return m, nil
}
