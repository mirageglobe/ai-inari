// model_router.go owns the root model's broadcast and background message
// handlers: window resize and theme changes that fan out to every view, plus
// system/stream messages (sysstats, title animation, connectivity, chat tokens).
// it does NOT own navigation (model_nav.go), input (model_input.go), or the
// top-level Update dispatch (model.go).

package tui

import (
	"math/rand"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/tui/views"
)

// updateBroadcast handles messages that must reach every view regardless of
// which one is active: terminal resize and theme changes.
func (m Model) updateBroadcast(msg tea.Msg) (Model, tea.Cmd, bool) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = ws.Width
		m.termHeight = ws.Height
		var cmds []tea.Cmd
		updated, cmd := m.agents.Update(ws)
		m.agents = updated.(views.Agents)
		cmds = append(cmds, cmd)
		updated2, cmd2 := m.models.Update(ws)
		m.models = updated2.(views.ModelSelector)
		cmds = append(cmds, cmd2)
		updated3, cmd3 := m.describe.Update(ws)
		m.describe = updated3.(views.Describe)
		cmds = append(cmds, cmd3)
		updated4, cmd4 := m.logs.Update(ws)
		m.logs = updated4.(views.Logs)
		cmds = append(cmds, cmd4)
		for id, chat := range m.chats {
			updated, cmd := chat.Update(ws)
			m.chats[id] = updated.(views.Chat)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...), true
	}
	if themeMsg, ok := msg.(views.ThemeChangedMsg); ok {
		var cmds []tea.Cmd
		updated, cmd := m.agents.Update(themeMsg)
		m.agents = updated.(views.Agents)
		cmds = append(cmds, cmd)
		updated2, cmd2 := m.models.Update(themeMsg)
		m.models = updated2.(views.ModelSelector)
		cmds = append(cmds, cmd2)
		updated3, cmd3 := m.describe.Update(themeMsg)
		m.describe = updated3.(views.Describe)
		cmds = append(cmds, cmd3)
		updated4, cmd4 := m.logs.Update(themeMsg)
		m.logs = updated4.(views.Logs)
		cmds = append(cmds, cmd4)
		for id, chat := range m.chats {
			updated, cmd := chat.Update(themeMsg)
			m.chats[id] = updated.(views.Chat)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...), true
	}
	return m, nil, false
}

// updateSystem handles background and stream messages: system stats polling, the
// title sweep animation, connectivity changes, and per-session token/done events
// routed by session ID so background sessions keep accumulating tokens.
func (m Model) updateSystem(msg tea.Msg) (Model, tea.Cmd, bool) {
	if stats, ok := msg.(views.SysStatsMsg); ok {
		m.sysStats = stats
		return m, views.SysStatsTick(), true
	}
	if _, ok := msg.(views.TitleStartMsg); ok {
		if rand.Intn(2) == 0 {
			m.titleDir = 1
			m.titleColorIdx = 0
		} else {
			m.titleDir = -1
			m.titleColorIdx = views.TitleLen - 1
		}
		return m, views.TitleTick(), true
	}
	if _, ok := msg.(views.TitleTickMsg); ok {
		m.titleColorIdx += m.titleDir
		// ray has fully exited: right edge (forward) or left edge (reverse, centre < -2)
		offScreen := m.titleColorIdx >= views.TitleLen || m.titleColorIdx < -2
		if offScreen {
			m.titleColorIdx = -10
			return m, views.TitlePause(), true
		}
		return m, views.TitleTick(), true
	}
	if conn, ok := msg.(views.ConnStatusMsg); ok {
		wasOffline := !m.connOnline
		m.connOnline = conn.OK
		offline := !conn.OK
		m.agents = m.agents.WithOffline(offline)
		m.describe = m.describe.WithOffline(offline)
		for id, chat := range m.chats {
			m.chats[id] = chat.WithOffline(offline)
		}
		if conn.OK {
			m.connErr = ""
			if wasOffline {
				// daemon just came back online; refresh sessions and running models now.
				return m, tea.Batch(views.ConnTick(m.client), m.agents.Init()), true
			}
		} else {
			m.connErr = "connection failed"
		}
		return m, views.ConnTick(m.client), true
	}
	if _, ok := msg.(views.IdleHintTickMsg); ok {
		// single root-owned idle poll: fan out to every chat, then reschedule.
		var cmds []tea.Cmd
		for id, chat := range m.chats {
			updated, cmd := chat.Update(msg)
			m.chats[id] = updated.(views.Chat)
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, views.IdleHintTick())
		return m, tea.Batch(cmds...), true
	}
	if tok, ok := msg.(views.ChatTokenMsg); ok {
		if chat, exists := m.chats[tok.SessionID]; exists {
			updated, cmd := chat.Update(tok)
			m.chats[tok.SessionID] = updated.(views.Chat)
			return m, cmd, true
		}
		return m, nil, true
	}
	if done, ok := msg.(views.ChatDoneMsg); ok {
		if chat, exists := m.chats[done.SessionID]; exists {
			updated, cmd := chat.Update(done)
			m.chats[done.SessionID] = updated.(views.Chat)
			return m, cmd, true
		}
		return m, nil, true
	}
	return m, nil, false
}
