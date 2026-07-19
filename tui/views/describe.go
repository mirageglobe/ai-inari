// describe.go owns the Describe view's struct, construction, Init, and
// small helpers. it does NOT own Update (describe_update.go) or rendering
// (describe_render.go).

package views

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

type describeHistoryMsg struct {
	count int
	err   error
}

type describeSetContextMsg struct {
	prompt string
	err    error
}

// Describe shows full metadata for the selected session and lets the user
// set a system prompt (context) that is prepended to every chat request for that session.
type Describe struct {
	client       *ipc.Client
	sessID       string
	sessName     string
	model        string
	systemPrompt string
	vram         int64
	msgCount     int
	fetched      bool
	viewport     viewport.Model
	input        textarea.Model
	ready        bool
	editing      bool
	saving       bool
	saveErr      string
	width        int
	height       int
	offline      bool
}

func NewDescribe() Describe {
	// input must be initialised via textarea.New() — a zero-value textarea panics on SetWidth/SetHeight.
	// dimensions are set correctly on the first WindowSizeMsg.
	return Describe{input: newContextInput("", 0, 0)}
}

// WithOffline updates the offline status of the Describe view.
func (d Describe) WithOffline(offline bool) Describe {
	d.offline = offline
	return d
}

// ForSession returns a copy of Describe configured for the given session.
// resets fetched and edit state so Init will re-fetch history count for the new session.
func (d Describe) ForSession(sess ipc.SessionInfo, vram int64, client *ipc.Client) Describe {
	d.client = client
	d.sessID = sess.ID
	d.sessName = sess.Name
	d.model = sess.Model
	d.systemPrompt = sess.SystemPrompt
	d.vram = vram
	d.msgCount = 0
	d.fetched = false
	d.editing = false
	d.saving = false
	d.saveErr = ""
	d.input = newContextInput(sess.SystemPrompt, d.width, d.height)
	return d
}

// IsEditing reports whether the describe view is in context-editing mode.
// model.go checks this to decide whether esc should cancel the edit or navigate back.
func (d Describe) IsEditing() bool { return d.editing }

func newContextInput(initial string, width, height int) textarea.Model {
	ta := textarea.New()
	ta.SetValue(initial)
	ta.Placeholder = "enter a system prompt for this session…"
	ta.CharLimit = 4096
	ta.SetWidth(max(width-2, 20))
	ta.SetHeight(max(height-5, 3))
	return ta
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (d Describe) Init() tea.Cmd {
	if d.sessID == "" || d.fetched {
		return nil
	}
	return fetchDescribeHistory(d.client, d.sessID)
}

func fetchDescribeHistory(client *ipc.Client, sessID string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := client.History(sessID)
		if err != nil {
			return describeHistoryMsg{err: err}
		}
		return describeHistoryMsg{count: len(msgs)}
	}
}

func saveContext(client *ipc.Client, sessID, prompt string) tea.Cmd {
	return func() tea.Msg {
		return describeSetContextMsg{prompt: prompt, err: client.SetContext(sessID, prompt)}
	}
}
