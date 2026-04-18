// Package tui is the root Bubble Tea model for fox (Terminal User Interface).
// It owns view routing — herd, models, logs, describe, and chat — and delegates input and rendering to each.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/tui/views"
)

type view int

const (
	viewHerd view = iota
	viewModels
	viewLogs
	viewDescribe
	viewChat
)

// Model is the root Bubble Tea model.
// activeSession holds the session ID (not name) of the currently open chat.
// chats is keyed by session ID so each session retains its display history across view switches.
type Model struct {
	client        *ipc.Client
	current       view
	returnView    view   // view to restore after model selector closes
	activeSession string // session ID of the currently open chat
	herd          views.Herd
	models        views.ModelSelector
	logs          views.Logs
	describe      views.Describe
	chats         map[string]views.Chat // keyed by session ID
	sysStats      views.SysStatsMsg
	connErr       string
	termWidth     int
	termHeight    int
}

func New(client *ipc.Client) Model {
	return Model{
		client:   client,
		current:  viewHerd,
		herd:     views.NewHerd(client),
		models:   views.NewModelSelector(client),
		logs:     views.NewLogs(),
		describe: views.NewDescribe(),
		chats:    make(map[string]views.Chat),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.herd.Init(), views.FetchSysStatsNow(), views.CheckConnNow(m.client))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Broadcast window size to all views that own a viewport so they size correctly
	// on startup and on terminal resize, regardless of which view is currently active.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = ws.Width
		m.termHeight = ws.Height
		var cmds []tea.Cmd
		updated, cmd := m.herd.Update(ws)
		m.herd = updated.(views.Herd)
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
		return m, tea.Batch(cmds...)
	}

	if stats, ok := msg.(views.SysStatsMsg); ok {
		m.sysStats = stats
		return m, views.SysStatsTick()
	}

	if conn, ok := msg.(views.ConnStatusMsg); ok {
		if conn.OK {
			m.connErr = ""
		} else {
			m.connErr = "connection failed"
		}
		return m, views.ConnTick(m.client)
	}

	if _, ok := msg.(views.BackToHerdMsg); ok {
		m.current = viewHerd
		return m, m.herd.Init()
	}

	// Open model selector targeting a specific session.
	if openMs, ok := msg.(views.OpenModelSelectorMsg); ok {
		m.returnView = m.current
		m.models = m.models.ForSession(openMs.SessionID, openMs.SessionName)
		m.current = viewModels
		return m, m.models.Init()
	}

	// A model was assigned to a session — herd handles the optimistic update and the assign RPC.
	if assign, ok := msg.(views.AssignModelMsg); ok {
		updated, cmd := m.herd.Update(assign)
		m.herd = updated.(views.Herd)
		if m.returnView == viewChat {
			m.current = viewChat
			return m, tea.Batch(cmd, m.chats[m.activeSession].Init())
		}
		m.current = viewHerd
		return m, cmd
	}

	// Open a session's chat.
	if sel, ok := msg.(views.SelectModelMsg); ok {
		m.activeSession = sel.SessionID
		if _, exists := m.chats[sel.SessionID]; !exists {
			chat := views.NewChat(m.client, sel.SessionID, sel.SessionName, sel.ModelName)
			// Size the viewport immediately with the known terminal dimensions so the
			// chat is ready before it ever receives a WindowSizeMsg.
			if m.termWidth > 0 && m.termHeight > 0 {
				sized, _ := chat.Update(tea.WindowSizeMsg{Width: m.termWidth, Height: m.termHeight})
				chat = sized.(views.Chat)
			}
			m.chats[sel.SessionID] = chat
		}
		m.current = viewChat
		return m, m.chats[sel.SessionID].Init()
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		if m.current == viewChat {
			switch key.String() {
			case "esc":
				m.current = viewHerd
				return m, m.herd.Init()
			case "ctrl+o":
				m.returnView = viewChat
				if chat, ok := m.chats[m.activeSession]; ok {
					m.models = m.models.ForSession(chat.SessionID(), chat.SessionName())
				}
				m.current = viewModels
				return m, m.models.Init()
			}
		} else {
			switch key.String() {
			case "q":
				return m, tea.Quit
			case "l":
				if m.current != viewModels {
					m.current = viewLogs
					return m, m.logs.Init()
				}
			case "d":
				m.current = viewDescribe
				return m, nil
			case "esc":
				m.current = viewHerd
				return m, m.herd.Init()
			}
		}
	}

	switch m.current {
	case viewHerd:
		updated, cmd := m.herd.Update(msg)
		m.herd = updated.(views.Herd)
		return m, cmd
	case viewModels:
		updated, cmd := m.models.Update(msg)
		m.models = updated.(views.ModelSelector)
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

func (m Model) View() string {
	header := views.RenderHeader(m.connErr) + "\n"
	bar := views.RenderSysBar(m.sysStats) + "\n"

	switch m.current {
	case viewModels:
		return header + bar + m.models.View()
	case viewLogs:
		return header + bar + m.logs.View()
	case viewDescribe:
		return header + bar + m.describe.View()
	case viewChat:
		return header + bar + m.chats[m.activeSession].View()
	default:
		return header + bar + m.herd.View()
	}
}
