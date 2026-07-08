// this file owns the Agents type, its Init/Update/View methods, and the hint list.
// message types, commands, and helpers live in agents_cmds.go.

package views

import (
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
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
// input is the command entry field shown in the footer; activated when the user types "/".
type Agents struct {
	client          *ipc.Client
	table           table.Model
	spinner         spinner.Model
	input           textinput.Model
	inputFocused    bool
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
	ti := textinput.New()
	ti.Placeholder = "type /command  (e.g. /agent add  /agent chat  /model select  /quit)"
	ti.Prompt = "❯ "
	ti.CharLimit = 64
	ti.ShowSuggestions = true
	ti.SetSuggestions(agentsCommands)
	return Agents{
		client:      client,
		table:       t,
		spinner:     s,
		input:       ti,
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
		ApplyTableStyles(&h.table)
		h.spinner.Style = spinnerStyle
		return h, nil

	case ThemeSaveErrMsg:
		h.status = connErrStyle.Render("theme save failed: " + msg.Err.Error())
		return h, nil

	case tea.WindowSizeMsg:
		h.width = msg.Width
		h.height = msg.Height
		// pre-render the hint at the actual width to count its line height.
		// on narrow terminals (~80 chars) the hint wraps to 2 lines; using a fixed
		// reservation of 1 would cause a 1-line overflow that scrolls the alt screen
		// and pushes the root header off the top of the display.
		hintStr := RenderHint(agentsHints(false, false, h.offline), h.width)
		h.hintHeight = strings.Count(hintStr, "\n") + 1
		// topbar(1) + border-top(1) + col-header(1) + border-bottom(1) + sessionLine(1) + cwdLine(1) + statusLine(1) + input(1) + hint(hintHeight)
		tableHeight := msg.Height - 8 - h.hintHeight
		if tableHeight < 1 {
			tableHeight = 1
		}
		h.tableHeight = tableHeight
		h.table.SetHeight(tableHeight)
		// resize model column to fill available width.
		// fixed cols: indicator(2) + name(20) + vram(12) + status(16) + context(12) = 62
		// overhead: 6 cols × 2 cell padding + 2 border = 14; total fixed overhead = 76.
		modelColW := h.width - 76
		if modelColW < 10 {
			modelColW = 10
		}
		h.table.SetColumns([]table.Column{
			{Title: "", Width: 2},
			{Title: "name", Width: 20},
			{Title: "model", Width: modelColW},
			{Title: "vram", Width: 12},
			{Title: "status", Width: 16},
			{Title: "context", Width: 12},
		})
		return h, nil

	case spinner.TickMsg:
		if h.loading {
			var cmd tea.Cmd
			h.spinner, cmd = h.spinner.Update(msg)
			return h, cmd
		}
		return h, nil

	case sessionsMsg:
		h.loading = false
		if msg.err != nil {
			log.Printf("session fetch error: %v", msg.err)
		} else {
			h.status = ""
			h.sessions = msg.sessions
			if len(msg.sessions) == 0 && !h.autoCreated {
				h.autoCreated = true
				return h, createSessionCmd(h.client, "default agent")
			}
			// on first successful load, auto-open the first session that has a model.
			if h.autoOpen && len(msg.sessions) > 0 {
				h.autoOpen = false
				for _, s := range msg.sessions {
					if s.Model != "" {
						sess := s
						return h, func() tea.Msg {
							return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars, SystemPrompt: sess.SystemPrompt}
						}
					}
				}
				h.autoOpen = false
			}
		}
		h.rebuildTable()
		// fetch caps for any model not yet cached
		var cmds []tea.Cmd
		for _, s := range h.sessions {
			if s.Model != "" {
				if _, ok := h.modelCaps[s.Model]; !ok {
					cmds = append(cmds, fetchModelCapsCmd(h.client, s.Model))
				}
			}
		}
		return h, tea.Batch(cmds...)

	case modelCapsMsg:
		if h.modelCaps == nil {
			h.modelCaps = make(map[string][]string)
		}
		h.modelCaps[msg.model] = msg.caps
		h.rebuildTable()
		return h, nil

	case runningMsg:
		if msg.err != nil {
			log.Printf("running fetch error: %v", msg.err)
		}
		// refresh live stats for display only — sessions are user-created, not derived from running models.
		h.runningInfo = make(map[string]runningMeta, len(msg.models))
		for _, m := range msg.models {
			h.runningInfo[m.Name] = runningMeta{vram: m.SizeVRAM, expiry: m.ExpiresAt}
		}
		h.rebuildTable()
		return h, nil

	case createSessionResultMsg:
		if msg.err != nil {
			h.status = connErrStyle.Render("create failed: " + msg.err.Error())
			return h, nil
		}
		return h, fetchSessions(h.client)

	case deleteSessionResultMsg:
		if msg.err != nil {
			h.status = connErrStyle.Render("delete failed: " + msg.err.Error())
		} else {
			deletedIdx := -1
			for i, s := range h.sessions {
				if s.ID == msg.id {
					deletedIdx = i
					h.sessions = append(h.sessions[:i], h.sessions[i+1:]...)
					break
				}
			}
			h.rebuildTable()
			if deletedIdx >= 0 && len(h.sessions) > 0 {
				cur := deletedIdx
				if cur >= len(h.sessions) {
					cur = len(h.sessions) - 1
				}
				h.table.SetCursor(cur)
			}
		}
		return h, nil

	case assignModelResultMsg:
		if msg.err != nil {
			// revert optimistic local update on failure.
			h.status = connErrStyle.Render("assign failed: " + msg.err.Error())
			for i, s := range h.sessions {
				if s.ID == msg.id {
					h.sessions[i].Model = ""
					break
				}
			}
			h.rebuildTable()
			return h, nil
		}
		// refresh running info and fetch caps for the newly assigned model.
		var cmds []tea.Cmd
		cmds = append(cmds, fetchRunning(h.client))
		for _, s := range h.sessions {
			if s.ID == msg.id && s.Model != "" {
				if _, ok := h.modelCaps[s.Model]; !ok {
					cmds = append(cmds, fetchModelCapsCmd(h.client, s.Model))
				}
				break
			}
		}
		return h, tea.Batch(cmds...)

	case unassignModelResultMsg:
		if msg.err != nil {
			// revert optimistic local update on failure.
			h.status = connErrStyle.Render("unassign failed: " + msg.err.Error())
			return h, fetchSessions(h.client)
		}
		return h, nil

	case exportChatResultMsg:
		if msg.err != nil {
			h.infoMsg = connErrStyle.Render("[error] " + msg.err.Error())
		} else {
			h.infoMsg = modelsStyle.Render("[info] exported → " + msg.path)
		}
		return h, nil

	case AssignModelMsg:
		// optimistically update the local session so the table reflects the change immediately.
		// assignModelCmd fires concurrently to persist the assignment in inarid.
		sessionName := msg.SessionID
		for i, s := range h.sessions {
			if s.ID == msg.SessionID {
				h.sessions[i].Model = msg.ModelName
				sessionName = s.Name
				break
			}
		}
		h.rebuildTable()
		return h, assignModelCmd(h.client, msg.SessionID, sessionName, msg.ModelName)

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				h.table.MoveUp(3)
			case tea.MouseButtonWheelDown:
				h.table.MoveDown(3)
			case tea.MouseButtonLeft:
				// topbar(1) + box border-top(1) + col-header(1) = first data row at Y=3.
				// table body occupies Y=[3, 3+tableHeight-1]; border-bottom at 3+tableHeight;
				// footer (sessionLine, cwdLine, statusLine, input, hint) follows after.
				const tableBodyY = 3
				tableBodyEndY := tableBodyY + h.tableHeight - 1
				if msg.Y >= tableBodyY && msg.Y <= tableBodyEndY {
					// click in table body → update cursor, blur command input
					tableH := h.table.Height()
					cursor := h.table.Cursor()
					cursorVisRow := min(cursor, tableH)
					clickedVisIdx := msg.Y - tableBodyY
					newCursor := cursor + clickedVisIdx - cursorVisRow
					if newCursor >= 0 && newCursor < len(h.sessions) {
						h.table.SetCursor(newCursor)
					}
					if h.inputFocused {
						h.inputFocused = false
						h.input.Blur()
					}
				} else if msg.Y > tableBodyEndY && !h.inputFocused {
					// click in footer → focus command input
					h.inputFocused = true
					return h, h.input.Focus()
				}
			}
		}
		return h, nil

	case tea.KeyMsg:
		h.infoMsg = ""

		// "/" activates the command input when not already focused.
		if !h.inputFocused && msg.String() == "/" {
			h.input.SetValue("/")
			h.input.CursorEnd()
			h.inputFocused = true
			return h, h.input.Focus()
		}

		if h.inputFocused {
			switch msg.String() {
			case "enter":
				text := strings.TrimSpace(h.input.Value())
				h.input.Reset()
				h.inputFocused = false
				h.input.Blur()
				if strings.HasPrefix(text, "/") {
					return h.handleSlashCommand(text)
				}
				return h, nil
			case "esc":
				h.input.Reset()
				h.inputFocused = false
				h.input.Blur()
				return h, nil
			}
			var cmd tea.Cmd
			h.input, cmd = h.input.Update(msg)
			return h, cmd
		}

		// table navigation when input is not focused.
		switch msg.String() {
		case "enter":
			if !h.offline {
				idx := h.table.Cursor()
				if idx >= 0 && idx < len(h.sessions) {
					sess := h.sessions[idx]
					if sess.Model != "" {
						return h, func() tea.Msg {
							return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars, SystemPrompt: sess.SystemPrompt}
						}
					}
				}
			}
		case "q":
			if h.modal {
				return h, func() tea.Msg { return CloseAgentsModalMsg{} }
			}
		}
	}
	var cmd tea.Cmd
	h.table, cmd = h.table.Update(msg)
	return h, cmd
}

// WithActiveSession returns a copy of the Agents with the active chat session marked.
func (h Agents) InputFocused() bool { return h.inputFocused }

// WithModal returns a copy of the agents view with modal mode set, controlling
// whether the [q] back-to-chat hint is shown in the footer.
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
