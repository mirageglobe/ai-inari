// this file owns the Agents type, its Init/Update/View methods, and the hint list.
// message types, commands, and helpers live in agents_cmds.go.

package views

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

var agentsStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

var (
	connErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	modelsStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// Agents is the default session-list view.
// sessions are owned by inarid; inari fetches them on init and after mutations.
// runningInfo is supplementary — it annotates sessions with live VRAM/expiry data.
// the view is hotkey-only: model selection, export, logs, and describe all live in
// chat now, so there is no text input to focus here.
type Agents struct {
	client          *ipc.Client
	table           table.Model
	spinner         spinner.Model
	loading         bool
	status          string
	sessions        []ipc.SessionInfo
	runningInfo     map[string]runningMeta
	width           int
	height          int
	hintHeight      int // actual rendered hint line count; varies with terminal width
	tableHeight     int // stored so mouse handler can compute footer Y boundary
	offline         bool
	autoCreated     bool                // guards against duplicate default-session creation on concurrent fetches
	autoOpen        bool                // true on first load; fires SelectModelMsg to open chat if a ready session exists
	infoMsg         string              // transient message shown in the status line; cleared on next keypress
	modelCaps       map[string][]string // capability tags per model name, fetched lazily
	activeSessionID string              // session currently open in chat view; marked in the table
	modal           bool                // true while rendered as a popup overlay on top of chat
}

func NewAgents(client *ipc.Client) Agents {
	// model column is resized dynamically in WindowSizeMsg; 28 is a safe default before first resize.
	cols := []table.Column{
		{Title: "", Width: 2},
		{Title: "name", Width: 20},
		{Title: "model", Width: 28},
		{Title: "vram", Width: 12},
		{Title: "status", Width: 16},
		{Title: "context", Width: 12},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		// height is overridden on first WindowSizeMsg; 12 is a safe default before that arrives.
		table.WithHeight(12),
	)
	ApplyTableStyles(&t)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle
	return Agents{
		client:      client,
		table:       t,
		spinner:     s,
		loading:     true,
		autoOpen:    true,
		runningInfo: make(map[string]runningMeta),
	}
}

func (h Agents) Init() tea.Cmd {
	return tea.Batch(fetchSessions(h.client), fetchRunning(h.client), h.spinner.Tick)
}

func (h Agents) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ThemeChangedMsg:
		return h.onThemeChanged()
	case ThemeSaveErrMsg:
		return h.onThemeSaveErr(msg)
	case tea.WindowSizeMsg:
		return h.onWindowSize(msg)
	case spinner.TickMsg:
		return h.onTick(msg)
	case sessionsMsg:
		return h.onSessions(msg)
	case modelCapsMsg:
		return h.onModelCaps(msg)
	case runningMsg:
		return h.onRunning(msg)
	case createSessionResultMsg:
		return h.onCreate(msg)
	case deleteSessionResultMsg:
		return h.onDelete(msg)
	case assignModelResultMsg:
		return h.onAssignResult(msg)
	case unassignModelResultMsg:
		return h.onUnassignResult(msg)
	case exportChatResultMsg:
		return h.onExportResult(msg)
	case AssignModelMsg:
		return h.onAssignModel(msg)
	case tea.MouseMsg:
		return h.onMouse(msg)
	case tea.KeyMsg:
		h2, cmd, handled := h.handleKey(msg)
		if handled {
			return h2, cmd
		}
		h = h2
	}
	// unhandled messages (table navigation keys, etc.) go to the table.
	var cmd tea.Cmd
	h.table, cmd = h.table.Update(msg)
	return h, cmd
}

// Booting reports whether the agents view is still waiting on its initial
// session fetch to decide between auto-opening a session into chat or
// falling back to the agents table itself; the root model uses this to
// avoid painting the table for the single frame before that decision lands.
func (h Agents) Booting() bool { return h.loading || h.autoOpen }

// WithModal marks whether the agents view is being rendered via RenderModal
// (as a popup over chat) rather than full-screen; this only affects Update's
// [q]/[esc] handling, which closes the popup instead of doing nothing.
func (h Agents) WithModal(v bool) Agents {
	h.modal = v
	return h
}

func (h Agents) WithActiveSession(id string) Agents {
	h.activeSessionID = id
	h.rebuildTable()
	return h
}

// WithOffline returns a copy of the Agents view with the offline flag set.
func (h Agents) WithOffline(offline bool) Agents {
	h.offline = offline
	return h
}
