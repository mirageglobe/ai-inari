// model selector view: lists available Ollama models and assigns one to a session.
// this file also owns modelsMsg and fetchModels since the selector is their sole consumer.

package views

import (
	"fmt"
	"sort"
	"strings"

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
	CWD          string // non-empty when builtin tools are active for this session
	ContextChars int    // total message chars at open time, for token estimation
}

// BackToAgentsMsg is emitted to return to the agents view.
type BackToAgentsMsg struct{}

// AssignModelMsg is emitted when a loaded model is assigned to a session.
type AssignModelMsg struct {
	SessionID string
	ModelName string
}

// OpenModelSelectorMsg is emitted by agents to open the model selector for a session.
type OpenModelSelectorMsg struct {
	SessionID   string
	SessionName string
}

type loadModelMsg struct {
	name string
	err  error
}

// ModelSelector lists available Ollama models and lets the user assign one to a session.
// recommended holds curated models (SPEC.md §6.1) for the detected hardware tier that
// are not already pulled; they're appended to the table marked [pull] - selecting one
// triggers `ollama pull` via inarid instead of requiring the user to run it themselves.
// rowLocal mirrors the table rows 1:1: true for an already-available model, false for
// a recommended-but-not-pulled entry.
type ModelSelector struct {
	client            *ipc.Client
	table             table.Model
	spinner           spinner.Model
	loading           bool
	status            string
	targetSessionID   string
	targetSessionName string
	width             int
	tierGB            int
	recommended       []CuratedModel
	rowLocal          []bool
	pullProgress      <-chan provider.PullProgress
	pullErrc          <-chan error
	pullTarget        string
}

func NewModelSelector(client *ipc.Client) ModelSelector {
	// model column is resized dynamically in WindowSizeMsg; this default targets UIWidth.
	// overhead = 2 (agentsStyle border) + 2×2 (cell padding) + 12 (VRAM) = 18; model = UIWidth-18.
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
	return ModelSelector{client: client, table: t, spinner: s, tierGB: DetectTier(TotalMemBytes())}
}

// ForSession returns a copy of the selector targeting the given session.
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
		tableHeight := msg.Height - 6
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
			names := make([]string, len(msg.models))
			for i, model := range msg.models {
				names[i] = model.Name
			}
			m.recommended = RecommendedFor(m.tierGB, names)

			rows := make([]table.Row, 0, len(msg.models)+len(m.recommended))
			local := make([]bool, 0, cap(rows))
			for _, model := range msg.models {
				rows = append(rows, table.Row{model.Name, formatBytes(model.Size)})
				local = append(local, true)
			}
			for _, c := range m.recommended {
				rows = append(rows, table.Row{c.Model, c.Size + " [pull]"})
				local = append(local, false)
			}
			m.table.SetRows(rows)
			m.rowLocal = local
		}
		return m, nil

	case loadModelMsg:
		m.loading = false
		if msg.err != nil {
			m.status = connErrStyle.Render("load failed: " + msg.err.Error())
			return m, nil
		}
		if m.targetSessionID == "" {
			return m, func() tea.Msg { return BackToAgentsMsg{} }
		}
		id, name := m.targetSessionID, msg.name
		return m, func() tea.Msg { return AssignModelMsg{SessionID: id, ModelName: name} }

	case pullProgressMsg:
		if msg.model != m.pullTarget {
			return m, nil
		}
		m.status = modelsStyle.Render(pullStatusText(msg.model, msg.progress))
		return m, readNextPullUpdate(m.pullTarget, m.pullProgress, m.pullErrc)

	case pullDoneMsg:
		m.pullProgress, m.pullErrc, m.pullTarget = nil, nil, ""
		if msg.err != nil {
			m.loading = false
			m.status = connErrStyle.Render("pull failed: " + msg.err.Error())
			return m, nil
		}
		// pulled successfully; refresh the list and continue into the normal
		// load+assign flow, same as picking an already-available model.
		name := msg.model
		m.status = modelsStyle.Render("loading " + name + " → " + m.targetSessionName + "...")
		return m, tea.Batch(
			fetchModels(m.client),
			func() tea.Msg { return loadModelMsg{name: name, err: m.client.LoadModel(name)} },
		)

	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "l":
			if !m.loading {
				idx := m.table.Cursor()
				if row := m.table.SelectedRow(); len(row) > 0 && idx >= 0 && idx < len(m.rowLocal) {
					name, size := row[0], row[1]
					m.loading = true
					if m.rowLocal[idx] {
						m.status = modelsStyle.Render("loading " + name + " (" + size + ") → " + m.targetSessionName + "...")
						return m, tea.Batch(
							m.spinner.Tick,
							func() tea.Msg {
								return loadModelMsg{name: name, err: m.client.LoadModel(name)}
							},
						)
					}

					progress := make(chan provider.PullProgress, 8)
					errc := make(chan error, 1)
					go func() {
						err := m.client.PullModel(name, progress)
						errc <- err
						close(progress)
					}()
					m.pullProgress, m.pullErrc, m.pullTarget = progress, errc, name
					m.status = modelsStyle.Render("pulling " + name + "...")
					return m, tea.Batch(m.spinner.Tick, readNextPullUpdate(name, progress, errc))
				}
			}
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

const modalInnerW = 64 // inner width of the model selector modal (excl. border+padding)

// WithModalDimensions resizes the selector's table to fit the modal box.
// called once when the modal opens; the table keeps these dimensions until next WindowSizeMsg.
func (m ModelSelector) WithModalDimensions() ModelSelector {
	modelColW := modalInnerW - 16 // 12 vram + 2 cell-padding each side
	if modelColW < 10 {
		modelColW = 10
	}
	m.table.SetColumns([]table.Column{
		{Title: "model", Width: modelColW},
		{Title: "est. vram", Width: 12},
	})
	m.table.SetHeight(8)
	return m
}

// RenderModal renders the selector as a centred overlay for use on top of the agents view.
func (m ModelSelector) RenderModal(termWidth, termHeight int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary)
	secStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary)

	title := titleStyle.Render("model select")
	if m.targetSessionName != "" {
		title += "  " + secStyle.Render("→ "+m.targetSessionName)
	}

	hint := RenderHint([]HintCmd{H("[enter] assign/pull"), H("[esc] cancel")}, modalInnerW)

	var lines []string
	lines = append(lines, title)
	lines = append(lines, m.table.View())
	if m.status != "" {
		line := m.status
		if m.loading {
			line = m.spinner.View() + " " + line
		}
		lines = append(lines, line)
	}
	lines = append(lines, hint)

	boxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ActiveTheme.Primary).
		Padding(0, 1)

	box := boxStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(termWidth, termHeight, lipgloss.Center, lipgloss.Center, box)
}

// renderRecommended returns a short note about curated recommendations (SPEC.md
// §6.1) for the detected hardware tier; the models themselves are appended to
// the table above, marked [pull]. returns "" when there is nothing to recommend.
func (m ModelSelector) renderRecommended() string {
	if len(m.recommended) == 0 {
		return ""
	}
	headerStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
	return headerStyle.Render(fmt.Sprintf("models marked [pull] are recommended for your system (%dgb tier); [enter] downloads and assigns", m.tierGB))
}

func (m ModelSelector) View() string {
	viewLabel := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary).Render("models")
	if m.targetSessionName != "" {
		viewLabel += "  " + lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Bold(true).Render("→ "+m.targetSessionName)
	}
	hint := viewLabel + "  " + RenderHint([]HintCmd{H("[enter] assign/pull"), H("[esc] back"), HS(), H("[?] help")}, m.width-10)
	body := agentsStyle.Render(m.table.View())
	if rec := m.renderRecommended(); rec != "" {
		body += "\n" + rec
	}
	if m.status != "" {
		line := m.status
		if m.loading {
			line = m.spinner.View() + " " + line
		}
		body += "\n" + line
	}
	return body + "\n" + hint
}
