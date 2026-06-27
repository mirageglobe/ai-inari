// this file owns the Herd type, its Init/Update/View methods, and the hint list.
// message types, commands, and helpers live in herd_cmds.go.

package views

import (
	"log"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

var herdStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

var (
	connErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	modelsStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// Herd is the default session-list view.
// sessions are owned by inarid; fox fetches them on init and after mutations.
// runningInfo is supplementary — it annotates sessions with live VRAM/expiry data.
// input is the command entry field shown in the footer; activated when the user types "/".
type Herd struct {
	client        *ipc.Client
	table         table.Model
	spinner       spinner.Model
	input         textinput.Model
	inputFocused  bool
	loading       bool
	status        string
	sessions      []ipc.SessionInfo
	runningInfo   map[string]runningMeta
	width         int
	height        int
	hintHeight    int // actual rendered hint line count; varies with terminal width
	tableHeight   int // stored so mouse handler can compute footer Y boundary
	offline       bool
	autoCreated   bool // guards against duplicate default-session creation on concurrent fetches
	autoOpen         bool // true on first load; fires SelectModelMsg to open chat if a ready session exists
	foxInfo          string // transient message shown in the fox status line; cleared on next keypress
	modelCaps        map[string][]string // capability tags per model name, fetched lazily
	activeSessionID  string // session currently open in chat view; marked in the table
}

func NewHerd(client *ipc.Client) Herd {
	// model column is resized dynamically in WindowSizeMsg; 28 is a safe default before first resize.
	cols := []table.Column{
		{Title: "", Width: 2},
		{Title: "agents (kitsune)", Width: 20},
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
	ti.SetSuggestions(herdCommands)
	return Herd{
		client:      client,
		table:       t,
		spinner:     s,
		input:       ti,
		loading:     true,
		autoOpen:    true,
		runningInfo: make(map[string]runningMeta),
	}
}

func (h Herd) Init() tea.Cmd {
	return tea.Batch(fetchSessions(h.client), fetchRunning(h.client), h.spinner.Tick)
}

func (h Herd) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		hintStr := RenderHint(herdHints(false, false, h.offline), h.width)
		h.hintHeight = strings.Count(hintStr, "\n") + 1
		// topbar(1) + border-top(1) + col-header(1) + border-bottom(1) + foxline(1) + cwdLine(1) + statusLine(1) + input(1) + hint(hintHeight)
		tableHeight := msg.Height - 8 - h.hintHeight
		if tableHeight < 1 {
			tableHeight = 1
		}
		h.tableHeight = tableHeight
		h.table.SetHeight(tableHeight)
		// resize model column to fill available width.
		// fixed cols: indicator(2) + kitsune(20) + vram(12) + status(16) + context(12) = 62
		// overhead: 6 cols × 2 cell padding + 2 border = 14; total fixed overhead = 76.
		modelColW := h.width - 76
		if modelColW < 10 {
			modelColW = 10
		}
		h.table.SetColumns([]table.Column{
			{Title: "", Width: 2},
			{Title: "agents (kitsune)", Width: 20},
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
				return h, createSessionCmd(h.client, "default kitsune")
			}
			// on first successful load, auto-open the first session that has a model.
			if h.autoOpen && len(msg.sessions) > 0 {
				h.autoOpen = false
				for _, s := range msg.sessions {
					if s.Model != "" {
						sess := s
						return h, func() tea.Msg {
							return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars}
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
			h.foxInfo = connErrStyle.Render("[error] " + msg.err.Error())
		} else {
			h.foxInfo = modelsStyle.Render("[info] exported → " + msg.path)
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
		h.foxInfo = ""

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
							return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars}
						}
					}
				}
			}
		}
	}
	var cmd tea.Cmd
	h.table, cmd = h.table.Update(msg)
	return h, cmd
}

// WithActiveSession returns a copy of the Herd with the active chat session marked.
func (h Herd) InputFocused() bool { return h.inputFocused }

func (h Herd) WithActiveSession(id string) Herd {
	h.activeSessionID = id
	h.rebuildTable()
	return h
}

// WithOffline returns a copy of the Herd view with the offline flag set.
func (h Herd) WithOffline(offline bool) Herd {
	h.offline = offline
	return h
}

func (h Herd) handleSlashCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/agent add":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		name := pickFoxName(h.usedNames())
		return h, createSessionCmd(h.client, name)
	case "/model select":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		idx := h.table.Cursor()
		if idx >= 0 && idx < len(h.sessions) {
			sess := h.sessions[idx]
			return h, func() tea.Msg {
				return OpenModelSelectorMsg{SessionID: sess.ID, SessionName: sess.Name}
			}
		}
	case "/model unload":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		idx := h.table.Cursor()
		if idx >= 0 && idx < len(h.sessions) {
			sess := h.sessions[idx]
			if sess.Model != "" {
				h.sessions[idx].Model = ""
				h.rebuildTable()
				return h, unassignModelCmd(h.client, sess.ID, sess.Name, sess.Model)
			}
		}
	case "/chat":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		// open chat for the first session in the list regardless of cursor position.
		if len(h.sessions) > 0 {
			sess := h.sessions[0]
			if sess.Model != "" {
				return h, func() tea.Msg {
					return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars}
				}
			}
			h.foxInfo = modelsStyle.Render("[warn] default kitsune has no model assigned")
		}
	case "/agent chat":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		idx := h.table.Cursor()
		if idx >= 0 && idx < len(h.sessions) {
			sess := h.sessions[idx]
			if sess.Model != "" {
				return h, func() tea.Msg {
					return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars}
				}
			}
		}
	case "/agent delete":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		idx := h.table.Cursor()
		if idx >= 0 && idx < len(h.sessions) {
			id := h.sessions[idx].ID
			return h, deleteSessionCmd(h.client, id)
		}
	case "/agent export":
		idx := h.table.Cursor()
		if idx >= 0 && idx < len(h.sessions) {
			sess := h.sessions[idx]
			return h, exportChatCmd(h.client, sess.ID, sess.Name)
		}
	case "/refresh":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		h.status = ""
		h.loading = true
		return h, tea.Batch(fetchSessions(h.client), fetchRunning(h.client), h.spinner.Tick)
	case "/agent logs":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		return h, func() tea.Msg { return OpenLogsMsg{} }
	case "/agent describe":
		if h.offline {
			h.foxInfo = modelsStyle.Render("[warn] offline")
			return h, nil
		}
		return h, func() tea.Msg { return OpenDescribeMsg{} }
	case "/theme":
		return h, func() tea.Msg { return CycleThemeMsg{} }
	case "/help":
		return h, func() tea.Msg { return ToggleHelpMsg{} }
	case "/quit":
		return h, tea.Quit
	default:
		h.foxInfo = modelsStyle.Render("[warn] unknown command: " + cmd)
	}
	return h, nil
}

// herdCommands is the ordered list of valid slash commands used for autocomplete suggestions.
var herdCommands = []string{
	"/chat",
	"/agent add",
	"/model select",
	"/model unload",
	"/agent chat",
	"/agent delete",
	"/agent export",
	"/refresh",
	"/agent logs",
	"/agent describe",
	"/theme",
	"/help",
	"/quit",
}

// herdHints returns the default command hint list for the herd view.
// hasSession, hasModel, and offline control which items are enabled.
func herdHints(hasSession, _ /* hasModel */, offline bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	return []HintCmd{
		hc("/chat", !offline),
		hc("/agent", !offline),
		hc("/model", hasSession && !offline),
		HS(),
		hc("/refresh", !offline),
		H("/theme"),
		H("/help"),
		H("/quit"),
	}
}

// defaultHints returns the expanded /default sub-command hint list.
func defaultHints(hasDefault, offline bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	return []HintCmd{
		hc("/chat", hasDefault && !offline),
	}
}

// modelHints returns the expanded /model sub-command hint list shown when the user is typing /model.
func modelHints(hasSession, hasModel, offline bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	return []HintCmd{
		hc("/model select", hasSession && !offline),
		hc("/model unload", hasModel && !offline),
	}
}

// agentHints returns the expanded /agent sub-command hint list shown when the user is typing /agent.
func agentHints(hasSession, hasModel, offline bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	return []HintCmd{
		hc("/agent add", !offline),
		hc("/agent chat", hasModel && !offline),
		hc("/agent describe", hasSession && !offline),
		hc("/agent export", hasSession),
		hc("/agent logs", !offline),
		hc("/agent delete", hasSession && !offline),
	}
}

func (h Herd) View() string {
	idx := h.table.Cursor()
	hasSession := idx >= 0 && idx < len(h.sessions)
	hasModel := hasSession && h.sessions[idx].Model != ""

	sessionName := "kitsune"
	if hasSession {
		sessionName = h.sessions[idx].Name
	}

	model := "—"
	tokens := "—"
	cwd := ""
	if hasSession {
		sess := h.sessions[idx]
		if sess.Model != "" {
			model = sess.Model
		}
		if sess.ContextChars > 0 {
			tokens = fmtTokens(sess.ContextChars)
		}
		if sess.CWD != "" {
			cwd = sess.CWD
		}
	}

	sessionLine := RenderSessionLine("herd", sessionName, model, tokens)
	cwdLine := renderCWDLine(cwd)

	var statusContent string
	switch {
	case h.status != "" && h.foxInfo != "":
		statusContent = h.status + "  " + h.foxInfo
	case h.status != "":
		statusContent = h.status
	case h.foxInfo != "":
		statusContent = h.foxInfo
	}
	statusLine := renderStatusLine(statusContent)

	hasDefault := len(h.sessions) > 0 && h.sessions[0].Model != ""
	var hints []HintCmd
	switch {
	case strings.HasPrefix(h.input.Value(), "/chat"):
		hints = defaultHints(hasDefault, h.offline)
	case strings.HasPrefix(h.input.Value(), "/agent"):
		hints = agentHints(hasSession, hasModel, h.offline)
	case strings.HasPrefix(h.input.Value(), "/model"):
		hints = modelHints(hasSession, hasModel, h.offline)
	default:
		hints = herdHints(hasSession, hasModel, h.offline)
	}
	hintLine := RenderHint(hints, h.width)

	if h.loading {
		pad := lipgloss.NewStyle().PaddingTop(4).PaddingLeft(2)
		body := herdStyle.Render(pad.Render(h.spinner.View() + " fetching kitsune…"))
		return body + "\n" + renderFooter(sessionLine, cwdLine, statusLine, "", hintLine)
	}

	body := herdStyle.Render(h.table.View())
	return body + "\n" + renderFooter(sessionLine, cwdLine, statusLine, h.input.View(), hintLine)
}

func (h *Herd) rebuildTable() {
	sort.Slice(h.sessions, func(i, j int) bool {
		return h.sessions[i].Name < h.sessions[j].Name
	})
	rows := make([]table.Row, len(h.sessions))
	for i, s := range h.sessions {
		vram, status := "—", "—"
		if info, ok := h.runningInfo[s.Model]; ok {
			vram = formatBytes(info.vram)
			status = formatExpiry(info.expiry)
		} else if s.Model != "" {
			// model assigned but not currently loaded in ollama memory
			status = "sleeping"
		}
		model := s.Model
		if model == "" {
			model = "—"
		} else if caps, ok := h.modelCaps[s.Model]; ok {
			for _, c := range caps {
				switch c {
				case "tools":
					model += " [tool]"
				case "vision":
					model += " [vis]"
				}
			}
		}
		ctx := "—"
		if s.ContextChars > 0 {
			ctx = fmtTokens(s.ContextChars)
		}
		indicator := " "
		if s.ID == h.activeSessionID {
			indicator = "▶"
		}
		rows[i] = table.Row{indicator, s.Name, model, vram, status, ctx}
	}
	h.table.SetRows(rows)
}


// SelectedSession returns the session at the current cursor plus its vram.
// returns false if no session is under the cursor.
func (h Herd) SelectedSession() (ipc.SessionInfo, int64, bool) {
	idx := h.table.Cursor()
	if idx < 0 || idx >= len(h.sessions) {
		return ipc.SessionInfo{}, 0, false
	}
	sess := h.sessions[idx]
	return sess, h.runningInfo[sess.Model].vram, true
}

func (h Herd) usedNames() []string {
	names := make([]string, len(h.sessions))
	for i, s := range h.sessions {
		names[i] = s.Name
	}
	return names
}
