// Package tui is the root Bubble Tea model for fox (Terminal User Interface).
// it owns view routing — herd, models, logs, describe, and chat — and delegates input and rendering to each.
package tui

import (
	"math/rand"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/tui/views"
)

type view int

const (
	viewHerd view = iota
	viewModels
	viewLogs
	viewDescribe
	viewChat
)

// Model is the root Bubble Tea model.
// activeSession holds the session ID (not name) of the currently open chat.
// chats is keyed by session ID so each session retains its display history across view switches.
type Model struct {
	client        *ipc.Client
	current       view
	returnView    view   // view to restore after model selector closes
	activeSession string // session ID of the currently open chat
	herd          views.Herd
	models        views.ModelSelector
	logs          views.Logs
	describe      views.Describe
	chats         map[string]views.Chat // keyed by session ID
	sysStats      views.SysStatsMsg
	connErr       string
	connOnline    bool // tracks last known connection state to detect offline→online transitions
	termWidth     int
	termHeight    int
	titleColorIdx int  // current ray position; -10 = off-screen (resting between sweeps)
	titleDir      int  // +1 = left-to-right, -1 = right-to-left
	showHelp      bool // true while the [?] help overlay is visible
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
		return "herd"
	}
}

func New(client *ipc.Client) Model {
	return Model{
		client:        client,
		current:       viewHerd,
		herd:          views.NewHerd(client),
		models:        views.NewModelSelector(client),
		logs:          views.NewLogs(),
		describe:      views.NewDescribe(),
		chats:         make(map[string]views.Chat),
		titleColorIdx: -10, // off-screen until first sweep begins
	}
}

func (m Model) Init() tea.Cmd {
	// fire TitleStartMsg immediately so the first sweep begins on launch.
	firstSweep := func() tea.Msg { return views.TitleStartMsg{} }
	return tea.Batch(m.herd.Init(), views.FetchSysStatsNow(), views.CheckConnNow(m.client), firstSweep)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// broadcast window size to all views that own a viewport so they size correctly
	// on startup and on terminal resize, regardless of which view is currently active.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = ws.Width
		m.termHeight = ws.Height
		var cmds []tea.Cmd
		updated, cmd := m.herd.Update(ws)
		m.herd = updated.(views.Herd)
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
		for id, chat := range m.chats {
			m.chats[id] = chat.WithOffline(offline)
		}
		if conn.OK {
			m.connErr = ""
			if wasOffline {
				// daemon just came back online — refresh sessions and running models immediately.
				return m, tea.Batch(views.ConnTick(m.client), m.herd.Init())
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

	if _, ok := msg.(views.BackToHerdMsg); ok {
		m.current = viewHerd
		return m, m.herd.Init()
	}

	// open model selector targeting a specific session.
	if openMs, ok := msg.(views.OpenModelSelectorMsg); ok {
		m.returnView = m.current
		m.models = m.models.ForSession(openMs.SessionID, openMs.SessionName)
		m.current = viewModels
		return m, m.models.Init()
	}

	// a model was assigned to a session — herd handles the optimistic update and the assign RPC.
	if assign, ok := msg.(views.AssignModelMsg); ok {
		updated, cmd := m.herd.Update(assign)
		m.herd = updated.(views.Herd)
		if m.returnView == viewChat {
			m.current = viewChat
			return m, tea.Batch(cmd, m.chats[m.activeSession].Init())
		}
		m.current = viewHerd
		return m, cmd
	}

	// open a session's chat.
	if sel, ok := msg.(views.SelectModelMsg); ok {
		m.activeSession = sel.SessionID
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

	if key, ok := msg.(tea.KeyMsg); ok {
		// [?] toggles the help overlay from any view.
		if key.String() == "?" {
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
			case "esc":
				m.current = viewHerd
				return m, m.herd.Init()
			case "ctrl+o":
				m.returnView = viewChat
				if chat, ok := m.chats[m.activeSession]; ok {
					m.models = m.models.ForSession(chat.SessionID(), chat.SessionName())
				}
				m.current = viewModels
				return m, m.models.Init()
			}
		case viewHerd:
			switch key.String() {
			case "q":
				return m, tea.Quit
			case "l":
				m.current = viewLogs
				return m, m.logs.Init()
			case "d":
				if sess, vram, ok := m.herd.SelectedSession(); ok {
					m.describe = m.describe.ForSession(sess, vram, m.client)
				}
				m.current = viewDescribe
				return m, m.describe.Init()
			}
		default:
			// esc from secondary views returns to herd, except when describe is in edit mode
			if key.String() == "esc" && !(m.current == viewDescribe && m.describe.IsEditing()) {
				m.current = viewHerd
				return m, m.herd.Init()
			}
		}
	}

	switch m.current {
	case viewHerd:
		updated, cmd := m.herd.Update(msg)
		m.herd = updated.(views.Herd)
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

func (m Model) View() string {
	topBar := views.RenderTopBar(m.connErr, m.sysStats, m.termWidth, m.titleColorIdx) + "\n"

	var body string
	if m.showHelp {
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
			body = m.herd.View()
		}
	}

	full := topBar + body
	// pad every render to termHeight lines so Bubble Tea's cursor tracking stays
	// consistent when switching between views of different heights. Without this,
	// switching from a short view (models, describe) back to a tall one (herd)
	// positions the cursor mid-screen, causing the top lines including the header
	// to render into stale rows and appear invisible.
	if m.termHeight > 0 {
		if pad := m.termHeight - 1 - strings.Count(full, "\n"); pad > 0 {
			full += strings.Repeat("\n", pad)
		}
	}
	return full
}
