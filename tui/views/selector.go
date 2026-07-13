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

// UnassignModelMsg is emitted from the selector's [u] hotkey to clear the
// target session's assigned model (replaces the former /model unload command).
type UnassignModelMsg struct {
	SessionID   string
	SessionName string
}

// OpenModelSelectorMsg is emitted by agents to open the model selector for a session.
type OpenModelSelectorMsg struct {
	SessionID   string
	SessionName string
	Model       string // the session's currently-assigned model, if any (gates [u] unload)
}

type loadModelMsg struct {
	name string
	err  error
}

// deleteModelMsg reports the outcome of a disk delete triggered by [d]+[y].
type deleteModelMsg struct {
	name string
	err  error
}

// ModelSelector lists available Ollama models and lets the user assign one to a session.
// recommended holds every curated model (SPEC.md §6.1, all hardware tiers) that is not
// already pulled; they're appended to the table with a [pull] status - selecting one
// triggers `ollama pull` via inarid instead of requiring the user to run it themselves.
// each row shows model | status (downloaded/[pull]) | notes | est. vram. rowLocal/rowModel
// mirror the table rows 1:1: rowLocal is true for an already-available model, rowModel
// carries the model tag behind each row.
type ModelSelector struct {
	client            *ipc.Client
	table             table.Model
	spinner           spinner.Model
	loading           bool
	status            string
	targetSessionID   string
	targetSessionName string
	targetModel       string // session's currently-assigned model; gates and labels the [u] unload hotkey
	pendingDelete     string // model name armed for [d] delete, awaiting [y] confirm; "" when idle
	width             int
	tierGB            int
	localModels       []provider.Model // pulled models, sorted by name; source for the "downloaded" rows
	running           map[string]bool  // model names currently resident in memory (ListRunning); shown as "loaded"
	recommended       []CuratedModel
	rowLocal          []bool
	rowModel          []string // actual model tag per row (the model cell shows the bare name)
	pullProgress      <-chan provider.PullProgress
	pullErrc          <-chan error
	pullTarget        string
}

func NewModelSelector(client *ipc.Client) ModelSelector {
	// columns are recomputed on open/resize via refreshRows; this default targets the modal budget.
	cols := selectorColumns(ModalInnerW)
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

// ForSession returns a copy of the selector targeting the given session. model
// is the session's currently-assigned model (empty if none), used to gate [u].
func (m ModelSelector) ForSession(sessionID, sessionName, model string) ModelSelector {
	m.targetSessionID = sessionID
	m.targetSessionName = sessionName
	m.targetModel = model
	m.status = ""
	m.pendingDelete = ""
	return m
}

func (m ModelSelector) Init() tea.Cmd {
	// fetch the model list and the currently-loaded set together so rows can
	// show a "loaded" status for models resident in memory.
	return tea.Batch(fetchModels(m.client), fetchRunning(m.client))
}

// selectorColumns sizes the model-selector table to fit inner (the modal inner
// width). status and vram are fixed; model and notes split the remainder. the
// 8 accounts for 2 cells of bubbles-table padding across the 4 columns.
func selectorColumns(inner int) []table.Column {
	const statusW, vramW = 11, 10 // fits "downloaded" (10) and "est. vram" header (9)
	rest := inner - 8 - statusW - vramW
	if rest < 24 {
		rest = 24
	}
	notesW := rest * 55 / 100
	if notesW < 10 {
		notesW = 10
	}
	modelW := rest - notesW
	if modelW < 12 {
		modelW = 12
	}
	return []table.Column{
		{Title: "model", Width: modelW},
		{Title: "status", Width: statusW},
		{Title: "notes", Width: notesW},
		{Title: "est. vram", Width: vramW},
	}
}

// buildSelectorRows builds the table rows and the parallel rowLocal/rowModel
// slices. pulled models come first, statused "loaded" when resident in memory
// (present in running) else "downloaded"; curated models not yet local follow,
// marked "[pull]". notes come from the curated table (empty for non-curated
// local models) and are truncated to notesW.
func buildSelectorRows(local []provider.Model, recommended []CuratedModel, running map[string]bool, notesW int) (rows []table.Row, rowLocal []bool, rowModel []string) {
	rows = make([]table.Row, 0, len(local)+len(recommended))
	for _, mdl := range local {
		status := "downloaded"
		if running[mdl.Name] {
			status = "loaded"
		}
		rows = append(rows, table.Row{mdl.Name, status, truncateCell(curatedNotes(mdl.Name), notesW), formatBytes(mdl.Size)})
		rowLocal = append(rowLocal, true)
		rowModel = append(rowModel, mdl.Name)
	}
	for _, c := range recommended {
		rows = append(rows, table.Row{c.Model, "[pull]", truncateCell(c.Notes, notesW), c.Size})
		rowLocal = append(rowLocal, false)
		rowModel = append(rowModel, c.Model)
	}
	return rows, rowLocal, rowModel
}

// refreshRows recomputes columns for the current width and rebuilds the rows
// from localModels + recommended. called on model list load and on resize so
// notes stay truncated to the live column width.
func (m *ModelSelector) refreshRows() {
	cols := selectorColumns(modalInnerWidth(m.width))
	notesW := cols[2].Width
	rows, local, models := buildSelectorRows(m.localModels, m.recommended, m.running, notesW)
	m.table.SetColumns(cols)
	m.table.SetRows(rows)
	m.rowLocal = local
	m.rowModel = models
}
