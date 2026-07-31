// model_input.go owns the root model's late-stage routing: modal capture (model
// selector / sessions popup / theme-save error), keyboard handling (theme picker,
// help overlay, per-view keys), and the final fall-through to the active view.
// it does NOT own broadcast/system messages (model_router.go), navigation
// (model_nav.go), or the Update dispatch (model.go).

package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/inari/tui/views"
)

// activeViewInputFocused reports whether the currently active view is capturing
// text input (chat message box, sessions filter, describe context editor). when
// true, unmodified keys belong to that input, not to global hotkeys.
func (m Model) activeViewInputFocused() bool {
	switch m.current {
	case viewChat:
		chat, ok := m.chats[m.activeSession]
		return ok && chat.InputFocused()
	case viewSessions:
		return m.sessions.Filtering()
	default:
		return false
	}
}

// isBareKey reports whether k is an unmodified key (no ctrl/alt chord). bare keys
// are the ones a focused text input would consume, so global handling of them is
// suppressed while an input is focused; modifier chords never collide with typing.
func isBareKey(k tea.KeyMsg) bool {
	s := k.String()
	return !strings.HasPrefix(s, "ctrl+") && !strings.HasPrefix(s, "alt+")
}

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
	// route all remaining messages to sessions when it is overlaid on chat as a popup.
	if m.showSessions {
		updated, cmd := m.sessions.Update(msg)
		m.sessions = updated.(views.Sessions)
		return m, cmd, true
	}
	// route to the describe overlay while it is open. q/esc close it and reveal the
	// view underneath - except while editing the context field, where those keys
	// belong to the editor (esc exits edit mode), so the overlay stays open.
	if m.showDescribe {
		if key, ok := msg.(tea.KeyMsg); ok && !m.describe.IsEditing() && (key.String() == "q" || key.String() == "esc") {
			m.showDescribe = false
			return m, nil, true
		}
		updated, cmd := m.describe.Update(msg)
		m.describe = updated.(views.Describe)
		return m, cmd, true
	}
	// route to the logs overlay while it is open; q/esc close it and reveal the view underneath.
	if m.showLogs {
		if key, ok := msg.(tea.KeyMsg); ok && (key.String() == "q" || key.String() == "esc") {
			m.showLogs = false
			return m, nil, true
		}
		updated, cmd := m.logs.Update(msg)
		m.logs = updated.(views.Logs)
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
		updated, cmd := m.sessions.Update(saveErr)
		m.sessions = updated.(views.Sessions)
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
	// general focus-aware suppression: with no global overlay active, when the
	// active view is capturing text input, unmodified keys belong to that input, so
	// fall through to the view rather than matching any global bare-key hotkey.
	// modifier chords (ctrl/alt) never collide with typing and still work. this is
	// the general guard the ad-hoc ?/t workaround lacked; a future bare-key global
	// binding cannot shadow typing while an input is focused.
	if !m.showThemePicker && !m.showHelp && m.activeViewInputFocused() && isBareKey(key) {
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
		case "esc", "q":
			m.showThemePicker = false
		}
		return m, nil, true
	}

	// while help is open, q or esc closes it; all other keys are consumed.
	if m.showHelp {
		if key.String() == "esc" || key.String() == "q" {
			m.showHelp = false
		}
		return m, nil, true
	}

	if m.current == viewChat {
		if key.String() == "ctrl+o" {
			if chat, ok := m.chats[m.activeSession]; ok {
				m.models = m.models.ForSession(chat.SessionID(), chat.SessionName(), chat.Model())
			}
			m.showModelSelector = true
			m.models = m.models.WithModalDimensions()
			return m, m.models.Init(), true
		}
	}
	return m, nil, false
}

// updateActiveView forwards a message to whichever view is currently active.
// this is the terminal step of Update, so it always reports the message handled.
func (m Model) updateActiveView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current {
	case viewSessions:
		updated, cmd := m.sessions.Update(msg)
		m.sessions = updated.(views.Sessions)
		return m, cmd
	case viewChat:
		updated, cmd := m.chats[m.activeSession].Update(msg)
		m.chats[m.activeSession] = updated.(views.Chat)
		return m, cmd
	}
	return m, nil
}
