package views

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

// SelectModelMsg is emitted when the user picks a model to chat with.
type SelectModelMsg struct{ Name string }

// BackToHerdMsg is emitted after a model loads successfully to return to the herd view.
type BackToHerdMsg struct{}

type loadModelMsg struct{ err error }

// ModelSelector lists available Ollama models and lets the user load one.
type ModelSelector struct {
	client *ipc.Client
	table  table.Model
	status string
}

func NewModelSelector(client *ipc.Client) ModelSelector {
	cols := []table.Column{
		{Title: "Model", Width: 34},
		{Title: "Est. VRAM", Width: 10},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	return ModelSelector{client: client, table: t}
}

func (m ModelSelector) Init() tea.Cmd {
	return fetchModels(m.client)
}

func (m ModelSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case modelsMsg:
		if msg.err == nil {
			rows := make([]table.Row, len(msg.models))
			for i, model := range msg.models {
				rows[i] = table.Row{model.Name, formatBytes(model.Size)}
			}
			m.table.SetRows(rows)
		}
		return m, nil
	case loadModelMsg:
		if msg.err != nil {
			m.status = connErrStyle.Render("load failed: " + msg.err.Error())
			return m, nil
		}
		return m, func() tea.Msg { return BackToHerdMsg{} }
	case tea.KeyMsg:
		if msg.String() == "l" {
			if row := m.table.SelectedRow(); len(row) > 0 {
				name, size := row[0], row[1]
				m.status = modelsStyle.Render("loading " + name + " (" + size + ")...")
				return m, func() tea.Msg {
					return loadModelMsg{err: m.client.LoadModel(name)}
				}
			}
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m ModelSelector) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("MODELS")
	hint := lipgloss.NewStyle().Faint(true).Render("l/enter load  esc back")
	body := herdStyle.Render(m.table.View())
	if m.status != "" {
		body += "\n" + m.status
	}
	return header + "\n" + body + "\n" + hint
}
