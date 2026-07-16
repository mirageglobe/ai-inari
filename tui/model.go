package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/config"
	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/tui/views"
)

// agentsModalMarginW/H shrink the agents table so it renders as a popup, not a
// full-screen view, when opened from chat via /agents.
const (
	agentsModalMarginW = 12
	agentsModalMarginH = 6
)

type view int

const (
	viewAgents view = iota
	viewChat
)

// Model is the root Bubble Tea model.
// activeSession holds the session ID (not name) of the currently open chat.
// chats is keyed by session ID so each session retains its display history across view switches.
type Model struct {
	client            *ipc.Client
	current           view
	activeSession     string // session ID of the currently open chat
	agents            views.Agents
	models            views.ModelSelector
	logs              views.Logs
	describe          views.Describe
	chats             map[string]views.Chat // keyed by session ID
	sysStats          views.SysStatsMsg
	connErr           string
	connOnline        bool // tracks last known connection state to detect offline→online transitions
	termWidth         int
	termHeight        int
	titleColorIdx     int  // current ray position; -10 = off-screen (resting between sweeps)
	titleDir          int  // +1 = left-to-right, -1 = right-to-left
	showHelp          bool // true while the [?] help overlay is visible
	showThemePicker   bool // true while the /theme modal is visible
	showModelSelector bool // true while the model selector modal is overlaid on agents
	showAgents        bool // true while agents is overlaid on chat as a popup modal (opened via /agents)
	showLogs          bool // true while logs is overlaid as a popup modal (opened via /logs or [l])
	showDescribe      bool // true while describe is overlaid as a popup modal (opened via /describe or [d])
	themePickerIdx    int  // cursor position in the theme picker
	themeIdx          int  // index into views.Themes; current active theme
	configPath        string
}

// currentViewName maps the active view enum to the string key used by RenderHelpOverlay.
func (m Model) currentViewName() string {
	switch m.current {
	case viewChat:
		return "chat"
	default:
		return "agents"
	}
}

// New creates the root model. configPath is used to persist theme changes.
func New(client *ipc.Client, configPath string, themeIdx int) Model {
	return Model{
		client:        client,
		current:       viewAgents,
		agents:        views.NewAgents(client),
		models:        views.NewModelSelector(client),
		logs:          views.NewLogs(),
		describe:      views.NewDescribe(),
		chats:         make(map[string]views.Chat),
		titleColorIdx: -10, // off-screen until first sweep begins
		themeIdx:      themeIdx,
		configPath:    configPath,
	}
}

func (m Model) Init() tea.Cmd {
	// fire TitleStartMsg immediately so the first sweep begins on launch.
	firstSweep := func() tea.Msg { return views.TitleStartMsg{} }
	return tea.Batch(m.agents.Init(), views.FetchSysStatsNow(), views.CheckConnNow(m.client), firstSweep, views.IdleHintTick())
}

// Update is the root message dispatcher. each stage handles the messages it
// owns and reports whether it consumed the message; broadcast and system
// messages run first, then navigation, then modal capture and keys, and finally
// the message falls through to whichever view is active.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mm, cmd, done := m.updateBroadcast(msg); done {
		return mm, cmd
	}
	if mm, cmd, done := m.updateSystem(msg); done {
		return mm, cmd
	}
	if mm, cmd, done := m.updateNav(msg); done {
		return mm, cmd
	}
	if mm, cmd, done := m.updateModal(msg); done {
		return mm, cmd
	}
	if mm, cmd, done := m.updateKeys(msg); done {
		return mm, cmd
	}
	return m.updateActiveView(msg)
}

func (m Model) applyTheme(idx int) (tea.Model, tea.Cmd) {
	views.ApplyTheme(views.Themes[idx])
	// persist synchronously so a save failure can be surfaced; the write is a small
	// local file and the goroutine it replaces also raced on the shared config.
	var saveErr error
	if m.configPath != "" {
		cfg, err := config.Load(m.configPath)
		if err != nil {
			saveErr = err
		} else {
			cfg.Theme = views.Themes[idx].Name
			saveErr = cfg.Save(m.configPath)
		}
	}
	cmds := []tea.Cmd{func() tea.Msg { return views.ThemeChangedMsg{} }}
	if saveErr != nil {
		cmds = append(cmds, func() tea.Msg { return views.ThemeSaveErrMsg{Err: saveErr} })
	}
	return m, tea.Batch(cmds...)
}
