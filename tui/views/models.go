package views

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

// SelectModelMsg is emitted when the user opens a session for chat.
type SelectModelMsg struct {
	SessionID   string
	SessionName string // display name shown in the chat header
	ModelName   string
}

// BackToHerdMsg is emitted to return to the herd view.
type BackToHerdMsg struct{}

// AssignModelMsg is emitted when a loaded model is assigned to a session.
type AssignModelMsg struct {
	SessionID string
	ModelName string
}

// OpenModelSelectorMsg is emitted by herd to open the model selector for a session.
type OpenModelSelectorMsg struct {
	SessionID   string
	SessionName string
}

type loadModelMsg struct {
	name string
	err  error
}

// ModelSelector lists available Ollama models and lets the user assign one to a kitsune session.
type ModelSelector struct {
	client            *ipc.Client
	table             table.Model
	spinner           spinner.Model
	loading           bool
	status            string
	targetSessionID   string
	targetSessionName string
	width             int
}

func NewModelSelector(client *ipc.Client) ModelSelector {
	// Column widths sum to 88; with borders ≈ 92 chars total.
	cols := []table.Column{
		{Title: "Model", Width: 74},
		{Title: "Est. VRAM", Width: 12},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle
	return ModelSelector{client: client, table: t, spinner: s}
}

// ForSession returns a copy of the selector targeting the given kitsune session.
func (m ModelSelector) ForSession(sessionID, sessionName string) ModelSelector {
	m.targetSessionID = sessionID
	m.targetSessionName = sessionName
	m.status = ""
	return m
}

func (m ModelSelector) Init() tea.Cmd {
	return fetchModels(m.client)
}

func (m ModelSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

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
		m.loading = false
		if msg.err != nil {
			m.status = connErrStyle.Render("load failed: " + msg.err.Error())
			return m, nil
		}
		if m.targetSessionID == "" {
			return m, func() tea.Msg { return BackToHerdMsg{} }
		}
		id, name := m.targetSessionID, msg.name
		return m, func() tea.Msg { return AssignModelMsg{SessionID: id, ModelName: name} }

	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "l":
			if !m.loading {
				if row := m.table.SelectedRow(); len(row) > 0 {
					name, size := row[0], row[1]
					m.loading = true
					m.status = modelsStyle.Render("loading " + name + " (" + size + ") → " + m.targetSessionName + "...")
					return m, tea.Batch(
						m.spinner.Tick,
						func() tea.Msg {
							return loadModelMsg{name: name, err: m.client.LoadModel(name)}
						},
					)
				}
			}
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m ModelSelector) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("MODELS")
	if m.targetSessionName != "" {
		title += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render("→ "+m.targetSessionName)
	}
	hint := RenderHint([]HintCmd{H("[enter] assign to kitsune"), H("[esc] back")}, m.width)
	body := herdStyle.Render(m.table.View())
	if m.status != "" {
		line := m.status
		if m.loading {
			line = m.spinner.View() + " " + line
		}
		body += "\n" + line
	}
	return title + "\n" + body + "\n" + hint
}
