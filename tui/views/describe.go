package views

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

// Describe shows full metadata for the selected kitsune session and lets the user
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
}

func NewDescribe() Describe {
	// input must be initialised via textarea.New() — a zero-value textarea panics on SetWidth/SetHeight.
	// dimensions are set correctly on the first WindowSizeMsg.
	return Describe{input: newContextInput("", 0, 0)}
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

func (d Describe) buildContent() string {
	if d.sessID == "" {
		return "no session selected."
	}

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Width(12)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	faintStyle := lipgloss.NewStyle().Faint(true)

	row := func(label, value string) string {
		return labelStyle.Render(label) + valueStyle.Render(value)
	}

	msgCountStr := fmt.Sprintf("%d messages", d.msgCount)
	if !d.fetched {
		msgCountStr = "fetching…"
	}

	vramStr := "—"
	if d.vram > 0 {
		vramStr = formatBytes(d.vram)
	}

	modelStr := d.model
	if modelStr == "" {
		modelStr = "—"
	}

	var behaviorBlock string
	if d.systemPrompt == "" {
		behaviorBlock = labelStyle.Render("behavior") + "\n" + faintStyle.Render("— not set  ([e] to edit behavior)")
	} else {
		behaviorBlock = labelStyle.Render("behavior") + "\n" + valueStyle.Render(d.systemPrompt)
	}

	return row("name", d.sessName) + "\n" +
		row("id", d.sessID) + "\n" +
		row("model", modelStr) + "\n" +
		row("vram", vramStr) + "\n" +
		row("history", msgCountStr) + "\n" +
		behaviorBlock
}

func (d Describe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ThemeChangedMsg:
		if d.ready {
			d.viewport.SetContent(d.buildContent())
		}
		return d, nil

	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		// topbar(1) + header(1) + border-top(1) + border-bottom(1) + hint(1) = 5 reserved
		vpHeight := msg.Height - 5
		if vpHeight < 1 {
			vpHeight = 1
		}
		// subtract 2 for herdStyle NormalBorder so total width = UIWidth.
		if !d.ready {
			d.viewport = viewport.New(d.width-2, vpHeight)
			d.ready = true
		} else {
			d.viewport.Width = d.width - 2
			d.viewport.Height = vpHeight
		}
		d.viewport.SetContent(d.buildContent())
		d.input.SetWidth(max(d.width-2, 20))
		d.input.SetHeight(max(msg.Height-5, 3))
		return d, nil

	case describeHistoryMsg:
		if msg.err == nil {
			d.msgCount = msg.count
		}
		d.fetched = true
		if d.ready {
			d.viewport.SetContent(d.buildContent())
		}
		return d, nil

	case describeSetContextMsg:
		d.saving = false
		if msg.err != nil {
			d.saveErr = "save failed: " + msg.err.Error()
			return d, nil
		}
		d.systemPrompt = msg.prompt
		d.editing = false
		d.saveErr = ""
		if d.ready {
			d.viewport.SetContent(d.buildContent())
		}
		return d, nil

	case tea.KeyMsg:
		if d.editing {
			switch msg.String() {
			case "ctrl+s":
				if d.saving {
					return d, nil
				}
				d.saving = true
				d.saveErr = ""
				return d, saveContext(d.client, d.sessID, d.input.Value())
			case "esc":
				d.editing = false
				d.saveErr = ""
				return d, nil
			}
			var cmd tea.Cmd
			d.input, cmd = d.input.Update(msg)
			return d, cmd
		}
		if msg.String() == "e" && d.sessID != "" {
			d.input = newContextInput(d.systemPrompt, d.width, d.height)
			d.editing = true
			d.saveErr = ""
			return d, d.input.Focus()
		}
	}

	if d.ready && !d.editing {
		var cmd tea.Cmd
		d.viewport, cmd = d.viewport.Update(msg)
		return d, cmd
	}
	return d, nil
}

func (d Describe) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary).Render("describe")

	if d.editing {
		editLabel := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render("  editing behavior")
		var hintCmds []HintCmd
		if d.saving {
			hintCmds = []HintCmd{HD("[ctrl+s] saving…"), HD("[esc] cancel")}
		} else {
			hintCmds = []HintCmd{H("[ctrl+s] save"), H("[esc] cancel")}
		}
		if d.saveErr != "" {
			hintCmds = append(hintCmds, HS(), HD(d.saveErr))
		}
		hint := RenderHint(hintCmds, d.width)
		return header + editLabel + "\n" + d.input.View() + "\n" + hint
	}

	hint := RenderHint([]HintCmd{H("[e] edit behavior"), H("[esc] back"), HS(), H("[?] help")}, d.width)

	var body string
	if !d.ready {
		body = herdStyle.Render(lipgloss.NewStyle().Faint(true).Render("loading…"))
	} else {
		body = herdStyle.Render(d.viewport.View())
	}

	return header + "\n" + body + "\n" + hint
}
