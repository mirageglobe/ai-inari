package tui

import (
	"math/rand"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/config"
	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/tui/views"
)

type view int

const (
	viewAgents view = iota
	viewModels
	viewLogs
	viewDescribe
	viewChat
)

// Model is the root Bubble Tea model.
// activeSession holds the session ID (not name) of the currently open chat.
// chats is keyed by session ID so each session retains its display history across view switches.
type Model struct {
	client            *ipc.Client
	current           view
	returnView        view   // view to restore after model selector closes
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
	themePickerIdx    int  // cursor position in the theme picker
	themeIdx          int  // index into views.Themes; current active theme
	configPath        string
}

// currentViewName maps the active view enum to the string key used by RenderHelpOverlay.
func (m Model) currentViewName() string {
	switch m.current {
	case viewChat:
		return "chat"
	case viewModels:
		return "models"
	case viewLogs:
		return "logs"
	case viewDescribe:
		return "describe"
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
	return tea.Batch(m.agents.Init(), views.FetchSysStatsNow(), views.CheckConnNow(m.client), firstSweep)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// broadcast window size to all views that own a viewport so they size correctly
	// on startup and on terminal resize, regardless of which view is currently active.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = ws.Width
		m.termHeight = ws.Height
		var cmds []tea.Cmd
		updated, cmd := m.agents.Update(ws)
		m.agents = updated.(views.Agents)
		cmds = append(cmds, cmd)
		updated2, cmd2 := m.models.Update(ws)
		m.models = updated2.(views.ModelSelector)
		cmds = append(cmds, cmd2)
		updated3, cmd3 := m.describe.Update(ws)
		m.describe = updated3.(views.Describe)
		cmds = append(cmds, cmd3)
		updated4, cmd4 := m.logs.Update(ws)
		m.logs = updated4.(views.Logs)
		cmds = append(cmds, cmd4)
		for id, chat := range m.chats {
			updated, cmd := chat.Update(ws)
			m.chats[id] = updated.(views.Chat)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	if themeMsg, ok := msg.(views.ThemeChangedMsg); ok {
		var cmds []tea.Cmd
		updated, cmd := m.agents.Update(themeMsg)
		m.agents = updated.(views.Agents)
		cmds = append(cmds, cmd)
		updated2, cmd2 := m.models.Update(themeMsg)
		m.models = updated2.(views.ModelSelector)
		cmds = append(cmds, cmd2)
		updated3, cmd3 := m.describe.Update(themeMsg)
		m.describe = updated3.(views.Describe)
		cmds = append(cmds, cmd3)
		updated4, cmd4 := m.logs.Update(themeMsg)
		m.logs = updated4.(views.Logs)
		cmds = append(cmds, cmd4)
		for id, chat := range m.chats {
			updated, cmd := chat.Update(themeMsg)
			m.chats[id] = updated.(views.Chat)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	if stats, ok := msg.(views.SysStatsMsg); ok {
		m.sysStats = stats
		return m, views.SysStatsTick()
	}

	if _, ok := msg.(views.TitleStartMsg); ok {
		if rand.Intn(2) == 0 {
			m.titleDir = 1
			m.titleColorIdx = 0
		} else {
			m.titleDir = -1
			m.titleColorIdx = views.TitleLen - 1
		}
		return m, views.TitleTick()
	}

	if _, ok := msg.(views.TitleTickMsg); ok {
		m.titleColorIdx += m.titleDir
		// ray has fully exited: right edge (forward) or left edge (reverse, centre < -2)
		offScreen := m.titleColorIdx >= views.TitleLen || m.titleColorIdx < -2
		if offScreen {
			m.titleColorIdx = -10
			return m, views.TitlePause()
		}
		return m, views.TitleTick()
	}

	if conn, ok := msg.(views.ConnStatusMsg); ok {
		wasOffline := !m.connOnline
		m.connOnline = conn.OK
		offline := !conn.OK
		m.agents = m.agents.WithOffline(offline)
		m.describe = m.describe.WithOffline(offline)
		for id, chat := range m.chats {
			m.chats[id] = chat.WithOffline(offline)
		}
		if conn.OK {
			m.connErr = ""
			if wasOffline {
				// daemon just came back online — refresh sessions and running models immediately.
				return m, tea.Batch(views.ConnTick(m.client), m.agents.Init())
			}
		} else {
			m.connErr = "connection failed"
		}
		return m, views.ConnTick(m.client)
	}

	// route stream messages by session ID so background sessions accumulate tokens
	// even when the user has switched to a different view.
	if tok, ok := msg.(views.ChatTokenMsg); ok {
		if chat, exists := m.chats[tok.SessionID]; exists {
			updated, cmd := chat.Update(tok)
			m.chats[tok.SessionID] = updated.(views.Chat)
			return m, cmd
		}
		return m, nil
	}
	if done, ok := msg.(views.ChatDoneMsg); ok {
		if chat, exists := m.chats[done.SessionID]; exists {
			updated, cmd := chat.Update(done)
			m.chats[done.SessionID] = updated.(views.Chat)
			return m, cmd
		}
		return m, nil
	}

	if _, ok := msg.(views.BackToAgentsMsg); ok {
		m.showModelSelector = false
		m.current = viewAgents
		return m, m.agents.Init()
	}

	if _, ok := msg.(views.OpenLogsMsg); ok {
		m.current = viewLogs
		return m, m.logs.Init()
	}

	if _, ok := msg.(views.OpenDescribeMsg); ok {
		if sess, vram, ok := m.agents.SelectedSession(); ok {
			m.describe = m.describe.ForSession(sess, vram, m.client)
		}
		m.current = viewDescribe
		return m, m.describe.Init()
	}

	if _, ok := msg.(views.CycleThemeMsg); ok {
		m.themePickerIdx = m.themeIdx
		m.showThemePicker = true
		return m, nil
	}

	if _, ok := msg.(views.ToggleHelpMsg); ok {
		m.showHelp = !m.showHelp
		return m, nil
	}

	// open model selector targeting a specific session.
	if openMs, ok := msg.(views.OpenModelSelectorMsg); ok {
		m.models = m.models.ForSession(openMs.SessionID, openMs.SessionName)
		if m.current == viewAgents {
			// modal overlay — stay on agents, resize table to fit the modal box.
			m.showModelSelector = true
			m.models = m.models.WithModalDimensions()
			return m, m.models.Init()
		}
		// from chat (ctrl+o) — keep full-view behaviour.
		m.returnView = m.current
		m.current = viewModels
		return m, m.models.Init()
	}

	// a model was assigned to a session — agents handles the optimistic update and the assign RPC.
	if assign, ok := msg.(views.AssignModelMsg); ok {
		updated, cmd := m.agents.Update(assign)
		m.agents = updated.(views.Agents)
		if m.showModelSelector {
			m.showModelSelector = false
			return m, cmd
		}
		if m.returnView == viewChat {
			m.current = viewChat
			chat := m.chats[m.activeSession].WithModel(assign.ModelName)
			m.chats[m.activeSession] = chat
			return m, tea.Batch(cmd, chat.Init())
		}
		m.current = viewAgents
		return m, cmd
	}

	// open a session's chat.
	if sel, ok := msg.(views.SelectModelMsg); ok {
		m.activeSession = sel.SessionID
		m.agents = m.agents.WithActiveSession(sel.SessionID)
		if _, exists := m.chats[sel.SessionID]; !exists {
			chat := views.NewChat(m.client, sel.SessionID, sel.SessionName, sel.ModelName, sel.CWD, sel.ContextChars)
			// size the viewport immediately with the known terminal dimensions so the
			// chat is ready before it ever receives a WindowSizeMsg.
			if m.termWidth > 0 && m.termHeight > 0 {
				sized, _ := chat.Update(tea.WindowSizeMsg{Width: m.termWidth, Height: m.termHeight})
				chat = sized.(views.Chat)
			}
			m.chats[sel.SessionID] = chat
		}
		m.current = viewChat
		return m, m.chats[sel.SessionID].Init()
	}

	// route all remaining messages to the model selector when the modal is active.
	if m.showModelSelector {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
			m.showModelSelector = false
			return m, nil
		}
		updated, cmd := m.models.Update(msg)
		m.models = updated.(views.ModelSelector)
		return m, cmd
	}

	// route a theme-save failure to the active view so it shows in the status bar.
	if saveErr, ok := msg.(views.ThemeSaveErrMsg); ok {
		if m.current == viewChat {
			if chat, exists := m.chats[m.activeSession]; exists {
				updated, cmd := chat.Update(saveErr)
				m.chats[m.activeSession] = updated.(views.Chat)
				return m, cmd
			}
		}
		updated, cmd := m.agents.Update(saveErr)
		m.agents = updated.(views.Agents)
		return m, cmd
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		if m.showThemePicker {
			switch key.String() {
			case "up", "k":
				if m.themePickerIdx > 0 {
					m.themePickerIdx--
				}
			case "down", "j":
				if m.themePickerIdx < len(views.Themes)-1 {
					m.themePickerIdx++
				}
			case "enter":
				m.showThemePicker = false
				m.themeIdx = m.themePickerIdx
				return m.applyTheme(m.themeIdx)
			case "esc":
				m.showThemePicker = false
			}
			return m, nil
		}

		// [?] toggles help from non-agents, non-chat views; agents uses /help slash command.
		if key.String() == "?" && m.current != viewChat && m.current != viewAgents {
			m.showHelp = !m.showHelp
			return m, nil
		}

		// while help is open, only [esc] (or a second [?]) closes it; all other keys are consumed.
		if m.showHelp {
			if key.String() == "esc" {
				m.showHelp = false
			}
			return m, nil
		}

		switch m.current {
		case viewChat:
			switch key.String() {
			case "ctrl+o":
				m.returnView = viewChat
				if chat, ok := m.chats[m.activeSession]; ok {
					m.models = m.models.ForSession(chat.SessionID(), chat.SessionName())
				}
				m.current = viewModels
				return m, m.models.Init()
			}
		default:
			// esc from secondary views returns to agents, except when describe is in edit mode
			if key.String() == "esc" && !(m.current == viewDescribe && m.describe.IsEditing()) {
				m.current = viewAgents
				return m, m.agents.Init()
			}
		}
	}

	switch m.current {
	case viewAgents:
		updated, cmd := m.agents.Update(msg)
		m.agents = updated.(views.Agents)
		return m, cmd
	case viewModels:
		updated, cmd := m.models.Update(msg)
		m.models = updated.(views.ModelSelector)
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
		updated, cmd := m.chats[m.activeSession].Update(msg)
		m.chats[m.activeSession] = updated.(views.Chat)
		return m, cmd
	}

	return m, nil
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

func (m Model) View() string {
	topBar := views.RenderTopBar(m.connErr, m.sysStats, m.termWidth, m.titleColorIdx) + "\n"

	var body string
	if m.showModelSelector {
		body = m.models.RenderModal(m.termWidth, m.termHeight-1)
	} else if m.showThemePicker {
		body = views.RenderThemeOverlay(m.themePickerIdx, m.termWidth, m.termHeight-1)
	} else if m.showHelp {
		// -1 to leave the top bar row; Place fills the remaining rows.
		body = views.RenderHelpOverlay(m.currentViewName(), m.termWidth, m.termHeight-1)
	} else {
		switch m.current {
		case viewModels:
			body = m.models.View()
		case viewLogs:
			body = m.logs.View()
		case viewDescribe:
			body = m.describe.View()
		case viewChat:
			body = m.chats[m.activeSession].View()
		default:
			body = m.agents.View()
		}
	}

	// emit cursor shape once here; views no longer emit escape sequences themselves.
	cursorEsc := views.ResetCursor
	switch m.current {
	case viewChat:
		if chat, ok := m.chats[m.activeSession]; ok && chat.InputFocused() {
			cursorEsc = views.BlinkBarCursor
		}
	case viewAgents:
		if m.agents.InputFocused() {
			cursorEsc = views.BlinkBarCursor
		}
	}
	full := cursorEsc + topBar + body
	// pad every render to termHeight lines so Bubble Tea's cursor tracking stays
	// consistent when switching between views of different heights. Without this,
	// switching from a short view (models, describe) back to a tall one (agents)
	// positions the cursor mid-screen, causing the top lines including the header
	// to render into stale rows and appear invisible.
	if m.termHeight > 0 {
		if pad := m.termHeight - 1 - strings.Count(full, "\n"); pad > 0 {
			full += strings.Repeat("\n", pad)
		}
	}
	return full
}
