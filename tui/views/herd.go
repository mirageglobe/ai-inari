// Package views contains the individual screen views rendered by the fox TUI:
// Herd (session table), Logs (token stream), Describe (session metadata), and Chat (head-inari conversation).
package views

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/ollama"
)

var herdStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

var (
	connErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	modelsStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// foxAdjectives are paired with "Fox" to form session names like "Arctic Fox".
var foxAdjectives = []string{
	"Arctic", "Amber", "Ash", "Blaze", "Copper", "Crimson", "Dusk",
	"Ember", "Fire", "Frost", "Ghost", "Golden", "Jade", "Midnight",
	"Rusty", "Scarlet", "Shadow", "Silver", "Storm", "Swift", "Thunder",
	"Tundra", "Violet", "Wild",
}

// runningMeta holds live stats for a running model, used to populate VRAM/Status columns.
type runningMeta struct {
	vram   int64
	expiry string
}

type modelsMsg struct {
	models []ollama.Model
	err    error
}

type runningMsg struct {
	models []ollama.RunningModel
	err    error
}

type sessionsMsg struct {
	sessions []ipc.SessionInfo
	err      error
}

type createSessionResultMsg struct {
	session ipc.SessionInfo
	err     error
}

type deleteSessionResultMsg struct {
	id  string
	err error
}

type assignModelResultMsg struct {
	id  string
	err error
}

type unassignModelResultMsg struct {
	id  string
	err error
}

// Herd is the default session-list view.
// sessions are owned by inarid; fox fetches them on init and after mutations.
// runningInfo is supplementary — it annotates sessions with live VRAM/expiry data.
type Herd struct {
	client      *ipc.Client
	table       table.Model
	spinner     spinner.Model
	loading     bool
	status      string
	sessions    []ipc.SessionInfo
	runningInfo map[string]runningMeta
	width       int
	height      int
	hintHeight  int // actual rendered hint line count; varies with terminal width
}

func NewHerd(client *ipc.Client) Herd {
	// column widths sum to 88; with 5 cols × 2 padding = 10, plus herdStyle border 2 = 100 total.
	cols := []table.Column{
		{Title: "kitsune", Width: 20},
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
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle
	return Herd{
		client:      client,
		table:       t,
		spinner:     s,
		loading:     true,
		runningInfo: make(map[string]runningMeta),
	}
}

func (h Herd) Init() tea.Cmd {
	return tea.Batch(fetchSessions(h.client), fetchRunning(h.client), h.spinner.Tick)
}

func (h Herd) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width = msg.Width
		if h.width > UIWidth {
			h.width = UIWidth
		}
		h.height = msg.Height
		// pre-render the hint at the capped width to count its actual line height.
		// on narrow terminals (~80 chars) the hint wraps to 2 lines; using a fixed
		// reservation of 1 would cause a 1-line overflow that scrolls the alt screen
		// and pushes the root header off the top of the display.
		hintStr := RenderHint(herdHints(false, false), h.width)
		h.hintHeight = strings.Count(hintStr, "\n") + 1
		// topbar(1) + border-top(1) + col-header(1) + border-bottom(1) + hint(hintHeight)
		tableHeight := msg.Height - 4 - h.hintHeight
		if tableHeight < 1 {
			tableHeight = 1
		}
		h.table.SetHeight(tableHeight)
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
		}
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
		} else {
			h.sessions = append(h.sessions, msg.session)
			h.rebuildTable()
		}
		return h, nil

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
		// refresh running info so VRAM/status columns reflect the newly loaded model.
		return h, fetchRunning(h.client)

	case unassignModelResultMsg:
		if msg.err != nil {
			// revert optimistic local update on failure.
			h.status = connErrStyle.Render("unassign failed: " + msg.err.Error())
			return h, tea.Batch(fetchSessions(h.client))
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

	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			name := pickFoxName(h.usedNames())
			return h, createSessionCmd(h.client, name)
		case "m":
			idx := h.table.Cursor()
			if idx >= 0 && idx < len(h.sessions) {
				sess := h.sessions[idx]
				return h, func() tea.Msg {
					return OpenModelSelectorMsg{SessionID: sess.ID, SessionName: sess.Name}
				}
			}
		case "r":
			h.status = ""
			h.loading = true
			return h, tea.Batch(fetchSessions(h.client), fetchRunning(h.client), h.spinner.Tick)
		case "c", "enter":
			idx := h.table.Cursor()
			if idx >= 0 && idx < len(h.sessions) {
				sess := h.sessions[idx]
				if sess.Model != "" {
					return h, func() tea.Msg {
						return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, ContextChars: sess.ContextChars}
					}
				}
			}
		case "u":
			idx := h.table.Cursor()
			if idx >= 0 && idx < len(h.sessions) {
				sess := h.sessions[idx]
				if sess.Model != "" {
					// optimistically clear the model locally; cmd persists it in inarid.
					h.sessions[idx].Model = ""
					h.rebuildTable()
					return h, unassignModelCmd(h.client, sess.ID, sess.Name, sess.Model)
				}
			}
		case "x":
			idx := h.table.Cursor()
			if idx >= 0 && idx < len(h.sessions) {
				id := h.sessions[idx].ID
				return h, deleteSessionCmd(h.client, id)
			}
		}
	}
	var cmd tea.Cmd
	h.table, cmd = h.table.Update(msg)
	return h, cmd
}

// herdHints returns the command hint list for the herd view.
// hasSession and hasModel control which items are enabled.
func herdHints(hasSession, hasModel bool) []HintCmd {
	hc := func(label string, enabled bool) HintCmd { return HintCmd{Label: label, Enabled: enabled} }
	return []HintCmd{
		H("[s] new kitsune"),
		hc("[m] model", hasSession),
		hc("[u] unload", hasModel),
		hc("[c] chat", hasModel),
		hc("[x] delete", hasSession),
		HS(),
		H("[r] refresh"),
		H("[l] logs"),
		H("[d] describe"),
		H("[q] quit"),
	}
}

func (h Herd) View() string {
	idx := h.table.Cursor()
	hasSession := idx >= 0 && idx < len(h.sessions)
	hasModel := hasSession && h.sessions[idx].Model != ""

	hint := RenderHint(herdHints(hasSession, hasModel), h.width)

	if h.loading {
		pad := lipgloss.NewStyle().PaddingTop(4).PaddingLeft(2)
		body := herdStyle.Render(pad.Render(h.spinner.View() + " fetching kitsune…"))
		return body + "\n" + hint
	}

	body := herdStyle.Render(h.table.View())
	if h.status != "" {
		body += "\n" + h.status
	}
	return body + "\n" + hint
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
		}
		ctx := "—"
		if s.ContextChars > 0 {
			ctx = fmtTokens(s.ContextChars)
		}
		rows[i] = table.Row{s.Name, model, vram, status, ctx}
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

// pickFoxName returns a fox-themed name not already in use.
func pickFoxName(used []string) string {
	inUse := make(map[string]bool, len(used))
	for _, v := range used {
		inUse[v] = true
	}
	pool := make([]string, len(foxAdjectives))
	copy(pool, foxAdjectives)
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	for _, adj := range pool {
		name := adj + " Fox"
		if !inUse[name] {
			return name
		}
	}
	return fmt.Sprintf("Fox #%d", len(used)+1)
}

func fetchSessions(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		sessions, err := client.ListSessions()
		if err != nil {
			return sessionsMsg{err: err}
		}
		return sessionsMsg{sessions: sessions}
	}
}

func createSessionCmd(client *ipc.Client, name string) tea.Cmd {
	return func() tea.Msg {
		sess, err := client.CreateSession(name)
		return createSessionResultMsg{session: sess, err: err}
	}
}

func deleteSessionCmd(client *ipc.Client, id string) tea.Cmd {
	return func() tea.Msg {
		err := client.DeleteSession(id)
		return deleteSessionResultMsg{id: id, err: err}
	}
}

func unassignModelCmd(client *ipc.Client, sessionID, sessionName, model string) tea.Cmd {
	return func() tea.Msg {
		err := client.UnassignModel(sessionID)
		if err == nil {
			log.Printf("kitsune %q (%s): model unloaded ← %s", sessionName, sessionID, model)
		}
		return unassignModelResultMsg{id: sessionID, err: err}
	}
}

func assignModelCmd(client *ipc.Client, sessionID, sessionName, model string) tea.Cmd {
	return func() tea.Msg {
		err := client.AssignModel(sessionID, model)
		if err == nil {
			log.Printf("kitsune %q (%s): model assigned → %s", sessionName, sessionID, model)
		}
		return assignModelResultMsg{id: sessionID, err: err}
	}
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

func fetchRunning(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		models, err := client.ListRunning()
		if err != nil {
			return runningMsg{err: err}
		}
		return runningMsg{models: models}
	}
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(b)/float64(1<<20))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func formatExpiry(expiresAt string) string {
	t, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return "—"
	}
	d := time.Until(t).Round(time.Second)
	if d <= 0 {
		return "waking"
	}
	return fmt.Sprintf("in %s", d)
}
