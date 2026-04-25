// Package views — model selector view: lists available Ollama models and assigns one to a session.
// this file also owns modelsMsg and fetchModels since the selector is their sole consumer.
package views

import (
	"sort"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/provider"
)

type modelsMsg struct {
	models []provider.Model
	err    error
}

func fetchModels(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		names, err := client.ListModels()
		if err != nil {
			return modelsMsg{err: err}
		}
		return modelsMsg{models: names}
	}
}

// SelectModelMsg is emitted when the user opens a session for chat.
type SelectModelMsg struct {
	SessionID    string
	SessionName  string // display name shown in the chat header
	ModelName    string
	CWD          string // non-empty when filesystem tools are active for this session
	ContextChars int    // total message chars at open time, for token estimation
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
	// model column is resized dynamically in WindowSizeMsg; this default targets UIWidth.
	// overhead = 2 (herdStyle border) + 2×2 (cell padding) + 12 (VRAM) = 18; model = UIWidth-18.
	cols := []table.Column{
		{Title: "model", Width: UIWidth - 18},
		{Title: "est. vram", Width: 12},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	ApplyTableStyles(&t)
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
	case ThemeChangedMsg:
		ApplyTableStyles(&m.table)
		m.spinner.Style = spinnerStyle
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		// topbar(1) + models header(1) + border-top(1) + col-header(1) + border-bottom(1) + status(1) + hint(1) = 7 reserved
		tableHeight := msg.Height - 7
		if tableHeight < 1 {
			tableHeight = 1
		}
		m.table.SetHeight(tableHeight)
		// resize model column so total width = m.width (see NewModelSelector for overhead breakdown).
		modelColW := m.width - 18
		if modelColW < 10 {
			modelColW = 10
		}
		m.table.SetColumns([]table.Column{
			{Title: "model", Width: modelColW},
			{Title: "est. vram", Width: 12},
		})
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
			sort.Slice(msg.models, func(i, j int) bool {
				return msg.models[i].Name < msg.models[j].Name
			})
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
	title := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary).Render("models")
	if m.targetSessionName != "" {
		title += "  " + lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Bold(true).Render("→ "+m.targetSessionName)
	}
	hint := RenderHint([]HintCmd{H("[enter] assign to kitsune"), H("[esc] back"), HS(), H("[?] help")}, m.width)
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
