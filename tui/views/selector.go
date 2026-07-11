// model selector view: lists available Ollama models and assigns one to a session.
// this file also owns modelsMsg and fetchModels since the selector is their sole consumer.
// it does NOT own Update (selector_update.go) or rendering (selector_render.go).

package views

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

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
	SystemPrompt string // injected file-tree/project-context text, for the chat pre-context line
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
// recommended holds every curated model (SPEC.md §6.1, all hardware tiers) that is not
// already pulled; they're appended to the table marked [pull], labelled with their tier
// and role - selecting one triggers `ollama pull` via inarid instead of requiring the
// user to run it themselves. rowLocal/rowModel mirror the table rows 1:1: rowLocal is
// true for an already-available model, rowModel carries the real tag behind each row's
// (possibly decorated) display label.
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
	rowModel          []string // actual model tag per row; row[0] may carry a display-only tier/role suffix
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
