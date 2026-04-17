package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-sudama/internal/ipc"
	"github.com/mirageglobe/ai-sudama/tui/views"
)

type view int

const (
	viewHerd view = iota
	viewLogs
	viewDescribe
	viewChat
)

// Model is the root Bubble Tea model.
type Model struct {
	client  *ipc.Client
	current view
	herd    views.Herd
	logs    views.Logs
	describe views.Describe
	chat    views.Chat
}

func New(client *ipc.Client) Model {
	return Model{
		client:  client,
		current: viewHerd,
		herd:    views.NewHerd(client),
		logs:    views.NewLogs(),
		describe: views.NewDescribe(),
		chat:    views.NewChat(client),
	}
}

func (m Model) Init() tea.Cmd {
	return m.herd.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "l":
			m.current = viewLogs
			return m, nil
		case "d":
			m.current = viewDescribe
			return m, nil
		case "i":
			m.current = viewChat
			return m, nil
		case "esc":
			m.current = viewHerd
			return m, nil
		}
	}

	switch m.current {
	case viewHerd:
		updated, cmd := m.herd.Update(msg)
		m.herd = updated.(views.Herd)
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
		updated, cmd := m.chat.Update(msg)
		m.chat = updated.(views.Chat)
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	switch m.current {
	case viewLogs:
		return m.logs.View()
	case viewDescribe:
		return m.describe.View()
	case viewChat:
		return m.chat.View()
	default:
		return m.herd.View()
	}
}
