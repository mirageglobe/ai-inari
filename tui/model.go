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
type Model struct {
	client      *ipc.Client
	current     view
	activeModel string
	herd        views.Herd
	models      views.ModelSelector
	logs        views.Logs
	describe    views.Describe
	chats    map[string]views.Chat
	sysStats views.SysStatsMsg
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
	return tea.Batch(m.herd.Init(), views.FetchSysStatsNow())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Refresh sys stats and reschedule the next tick.
	if stats, ok := msg.(views.SysStatsMsg); ok {
		m.sysStats = stats
		return m, views.SysStatsTick()
	}

	if _, ok := msg.(views.BackToHerdMsg); ok {
		m.current = viewHerd
		return m, m.herd.Init()
	}

	// Handle model selection before key routing.
	if sel, ok := msg.(views.SelectModelMsg); ok {
		m.activeModel = sel.Name
		if _, exists := m.chats[sel.Name]; !exists {
			m.chats[sel.Name] = views.NewChat(m.client, sel.Name)
		}
		m.current = viewChat
		return m, m.chats[sel.Name].Init()
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		// Chat is insert mode (vim convention): only esc and ctrl+c are captured
		// globally so that typed words never trigger navigation shortcuts.
		if m.current == viewChat {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.current = viewHerd
				return m, m.herd.Init()
			}
		} else {
			// Navigation mode: letter shortcuts active.
			switch key.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "m":
				m.current = viewModels
				return m, m.models.Init()
			case "l":
				if m.current != viewModels {
					m.current = viewLogs
					return m, nil
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
		updated, cmd := m.chats[m.activeModel].Update(msg)
		m.chats[m.activeModel] = updated.(views.Chat)
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	bar := views.RenderSysBar(m.sysStats) + "\n"

	switch m.current {
	case viewModels:
		return bar + m.models.View()
	case viewLogs:
		return bar + m.logs.View()
	case viewDescribe:
		return bar + m.describe.View()
	case viewChat:
		return bar + m.chats[m.activeModel].View()
	default:
		return bar + m.herd.View()
	}
}
