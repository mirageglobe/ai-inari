package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type chatReplyMsg struct {
	text string
	err  error
}

// Chat is the interactive conversation view for a session.
// display holds the rendered lines shown in the viewport — local to this fox instance.
// All message history lives in inarid; fox sends only the new user text each turn.
type Chat struct {
	client      *ipc.Client
	sessionID   string
	sessionName string
	model       string // display only
	display     []string
	viewport    viewport.Model
	input       textarea.Model
	waiting     bool
	ready       bool
}

// Init re-focuses the textarea so typing works when resuming a session.
func (c Chat) Init() tea.Cmd { return c.input.Focus() }

func (c Chat) SessionID() string   { return c.sessionID }
func (c Chat) SessionName() string { return c.sessionName }

func NewChat(client *ipc.Client, sessionID, sessionName, model string) Chat {
	ta := textarea.New()
	ta.Placeholder = "Message " + sessionName + " (" + model + ")..."
	ta.Focus()
	ta.SetHeight(2)
	ta.CharLimit = 2048

	return Chat{
		client:      client,
		sessionID:   sessionID,
		sessionName: sessionName,
		model:       model,
		input:       ta,
	}
}

func (c Chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case chatReplyMsg:
		c.waiting = false
		// Remove the "thinking..." placeholder added when the message was sent.
		if len(c.display) > 0 {
			c.display = c.display[:len(c.display)-1]
		}
		if msg.err != nil {
			c.display = append(c.display, errorStyle.Render("error: "+msg.err.Error()))
		} else {
			c.display = append(c.display, assistantStyle.Render(c.sessionName+": ")+msg.text)
		}
		c.viewport.SetContent(strings.Join(c.display, "\n"))
		c.viewport.GotoBottom()
		return c, nil

	case tea.WindowSizeMsg:
		// model header(1) + sysbar(1) + chat header(1) + border-top(1) + border-bottom(1) + input(2) + hint(1) = 8 reserved
		// Viewport width shrinks by 2 for the border's left and right columns.
		height := msg.Height - 8
		if height < 1 {
			height = 1
		}
		if !c.ready {
			c.viewport = viewport.New(msg.Width-2, height)
			c.ready = true
		} else {
			c.viewport.Width = msg.Width - 2
			c.viewport.Height = height
		}
		c.viewport.SetContent(strings.Join(c.display, "\n"))
		c.viewport.GotoBottom()
		return c, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && !c.waiting {
			text := strings.TrimSpace(c.input.Value())
			if text == "" {
				return c, nil
			}
			c.display = append(c.display, userStyle.Render("you: ")+text)
			c.display = append(c.display, lipgloss.NewStyle().Faint(true).Render("thinking..."))
			c.viewport.SetContent(strings.Join(c.display, "\n"))
			c.viewport.GotoBottom()
			c.input.Reset()
			c.waiting = true
			return c, sendMessage(c.client, c.sessionID, text)
		}
	}

	var (
		vpCmd tea.Cmd
		taCmd tea.Cmd
	)
	c.viewport, vpCmd = c.viewport.Update(msg)
	c.input, taCmd = c.input.Update(msg)
	return c, tea.Batch(vpCmd, taCmd)
}

func (c Chat) View() string {
	title := c.model
	if c.sessionName != "" {
		title = c.sessionName + "  " + lipgloss.NewStyle().Faint(true).Render("("+c.model+")")
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("CHAT") +
		"  " + lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(title)
	hint := RenderHint([]HintCmd{
		H("[enter] send"),
		H("[ctrl+o] change model"),
		HS(),
		H("[esc] back"),
	}, c.viewport.Width)
	body := herdStyle.Render(c.viewport.View())
	return header + "\n" + body + "\n" + c.input.View() + "\n" + hint
}

func sendMessage(client *ipc.Client, sessionID, text string) tea.Cmd {
	return func() tea.Msg {
		reply, err := client.Chat(sessionID, text)
		if err != nil {
			return chatReplyMsg{err: err}
		}
		return chatReplyMsg{text: reply}
	}
}
