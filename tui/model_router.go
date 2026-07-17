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
// broadcast fans msg out to every sub-view (agents, model selector, describe, logs,
// and each chat), returning the accumulated commands. the single fan-out point, so a
// new view is wired here once instead of in each broadcast handler. note: the offline
// fan-out in updateSystem stays separate; it calls WithOffline, not Update.
func (m Model) broadcast(msg tea.Msg) (Model, []tea.Cmd) {
	var cmds []tea.Cmd
	a, c := m.agents.Update(msg)
	m.agents = a.(views.Agents)
	cmds = append(cmds, c)
	ms, c := m.models.Update(msg)
	m.models = ms.(views.ModelSelector)
	cmds = append(cmds, c)
	d, c := m.describe.Update(msg)
	m.describe = d.(views.Describe)
	cmds = append(cmds, c)
	l, c := m.logs.Update(msg)
	m.logs = l.(views.Logs)
	cmds = append(cmds, c)
	for id, chat := range m.chats {
		u, cc := chat.Update(msg)
		m.chats[id] = u.(views.Chat)
		cmds = append(cmds, cc)
	}
	return m, cmds
}

func (m Model) updateBroadcast(msg tea.Msg) (Model, tea.Cmd, bool) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = ws.Width
		m.termHeight = ws.Height
		mm, cmds := m.broadcast(ws)
		return mm, tea.Batch(cmds...), true
	}
	if _, ok := msg.(views.ThemeChangedMsg); ok {
		mm, cmds := m.broadcast(msg)
		return mm, tea.Batch(cmds...), true
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
