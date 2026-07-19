// model_nav.go owns the root model's navigation and command-routing handlers:
// the messages emitted by chat slash commands and popups that switch views,
// open modals, or select a session. it does NOT own broadcast/system messages
// (model_router.go), input (model_input.go), or the Update dispatch (model.go).

package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/tui/views"
)

// updateNav handles view-navigation and command messages. it returns handled
// false when the message is not one of these, so the caller continues routing.
func (m Model) updateNav(msg tea.Msg) (Model, tea.Cmd, bool) {
	if _, ok := msg.(views.BackToSessionsMsg); ok {
		m.showModelSelector = false
		m.current = viewSessions
		return m, m.sessions.Init(), true
	}
	// /sessions from chat opens sessions as a popup over chat rather than switching away.
	if _, ok := msg.(views.OpenSessionsMsg); ok {
		m.showSessions = true
		m.sessions = m.sessions.WithModal(true)
		if m.termWidth > 0 && m.termHeight > 0 {
			updated, _ := m.sessions.Update(tea.WindowSizeMsg{Width: m.termWidth - sessionsModalMarginW, Height: m.termHeight - sessionsModalMarginH})
			m.sessions = updated.(views.Sessions)
		}
		return m, m.sessions.Init(), true
	}
	// /chat from chat jumps to the default session's chat, regardless of which
	// session is currently active.
	if _, ok := msg.(views.OpenDefaultChatMsg); ok {
		if sess, ok := m.sessions.DefaultSession(); ok && sess.Model != "" {
			return m, func() tea.Msg {
				return views.SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars, SystemPrompt: sess.SystemPrompt}
			}, true
		}
		return m, nil, true
	}
	// /refresh from chat silently reloads the sessions session list in the
	// background so it is fresh next time /sessions is opened.
	if _, ok := msg.(views.RefreshSessionsMsg); ok {
		return m, m.sessions.Init(), true
	}
	// [q] inside the sessions popup closes it and restores chat.
	if _, ok := msg.(views.CloseSessionsModalMsg); ok {
		m.showSessions = false
		m.sessions = m.sessions.WithModal(false)
		if m.termWidth > 0 && m.termHeight > 0 {
			updated, _ := m.sessions.Update(tea.WindowSizeMsg{Width: m.termWidth, Height: m.termHeight})
			m.sessions = updated.(views.Sessions)
		}
		return m, nil, true
	}
	if _, ok := msg.(views.OpenLogsMsg); ok {
		m.showLogs = true
		return m, m.logs.Init(), true
	}
	if _, ok := msg.(views.OpenDescribeMsg); ok {
		if sess, vram, ok := m.sessions.SelectedSession(); ok {
			m.describe = m.describe.ForSession(sess, vram, m.client)
		}
		m.showDescribe = true
		return m, m.describe.Init(), true
	}
	if _, ok := msg.(views.CycleThemeMsg); ok {
		m.themePickerIdx = m.themeIdx
		m.showThemePicker = true
		return m, nil, true
	}
	if _, ok := msg.(views.ToggleHelpMsg); ok {
		m.showHelp = !m.showHelp
		return m, nil, true
	}
	// open model selector targeting a specific session, always as a modal overlay on
	// whatever view is current: sessions, sessions-over-chat, or chat directly.
	if openMs, ok := msg.(views.OpenModelSelectorMsg); ok {
		m.models = m.models.ForSession(openMs.SessionID, openMs.SessionName, openMs.Model)
		m.showModelSelector = true
		m.models = m.models.WithModalDimensions()
		return m, m.models.Init(), true
	}
	// a model was assigned to a session; sessions handles the optimistic update and the assign RPC.
	if assign, ok := msg.(views.AssignModelMsg); ok {
		updated, cmd := m.sessions.Update(assign)
		m.sessions = updated.(views.Sessions)
		m.showModelSelector = false
		if m.current == viewChat && !m.showSessions {
			// opened directly from chat, not via the sessions popup, so apply to the active chat.
			chat := m.chats[m.activeSession].WithModel(assign.ModelName)
			m.chats[m.activeSession] = chat
			return m, tea.Batch(cmd, chat.Init()), true
		}
		return m, cmd, true
	}
	// [u] in the selector cleared the session's model; sessions does the optimistic
	// update and the unassign RPC, mirroring the assign path.
	if un, ok := msg.(views.UnassignModelMsg); ok {
		updated, cmd := m.sessions.Update(un)
		m.sessions = updated.(views.Sessions)
		m.showModelSelector = false
		if m.current == viewChat && !m.showSessions {
			// opened directly from chat: clear the active chat's displayed model.
			chat := m.chats[m.activeSession].WithModel("")
			m.chats[m.activeSession] = chat
		}
		return m, cmd, true
	}
	// open a session's chat.
	if sel, ok := msg.(views.SelectModelMsg); ok {
		m.activeSession = sel.SessionID
		m.sessions = m.sessions.WithActiveSession(sel.SessionID)
		if m.showSessions {
			// selecting a session from the sessions popup closes it back to chat.
			m.showSessions = false
			m.sessions = m.sessions.WithModal(false)
			if m.termWidth > 0 && m.termHeight > 0 {
				updated, _ := m.sessions.Update(tea.WindowSizeMsg{Width: m.termWidth, Height: m.termHeight})
				m.sessions = updated.(views.Sessions)
			}
		}
		if _, exists := m.chats[sel.SessionID]; !exists {
			chat := views.NewChat(m.client, sel.SessionID, sel.SessionName, sel.ModelName, sel.CWD, sel.ContextChars, sel.NumCtxOverride, sel.SystemPrompt)
			// size the viewport immediately with the known terminal dimensions so the
			// chat is ready before it ever receives a WindowSizeMsg.
			if m.termWidth > 0 && m.termHeight > 0 {
				sized, _ := chat.Update(tea.WindowSizeMsg{Width: m.termWidth, Height: m.termHeight})
				chat = sized.(views.Chat)
			}
			m.chats[sel.SessionID] = chat
		}
		m.current = viewChat
		return m, m.chats[sel.SessionID].Init(), true
	}
	return m, nil, false
}
