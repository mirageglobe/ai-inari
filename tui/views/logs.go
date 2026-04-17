package views

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Logs tails the output of the selected session.
type Logs struct {
	viewport viewport.Model
}

func (l Logs) Init() tea.Cmd { return nil }

func NewLogs() Logs {
	vp := viewport.New(80, 20)
	vp.SetContent("")
	return Logs{viewport: vp}
}

func (l Logs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	l.viewport, cmd = l.viewport.Update(msg)
	return l, cmd
}

func (l Logs) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("LOGS")
	hint := lipgloss.NewStyle().Faint(true).Render("esc back")
	return header + "\n" + l.viewport.View() + "\n" + hint
}
