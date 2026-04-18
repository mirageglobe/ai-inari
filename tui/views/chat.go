package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/ollama"
)

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	thinkingStyle  = lipgloss.NewStyle().Faint(true)
)

type chatReplyMsg struct {
	text string
	err  error
}

type chatHistoryMsg struct {
	messages []ollama.Message
	err      error
}

// Chat is the interactive conversation view for a session.
// display holds the rendered lines shown in the viewport — local to this fox instance.
// All message history lives in inarid; fox sends only the new user text each turn.
// The waiting spinner is rendered separately and is never written into display.
type Chat struct {
	client      *ipc.Client
	sessionID   string
	sessionName string
	model       string // display only
	display     []string
	viewport    viewport.Model
	input       textarea.Model
	spinner     spinner.Model
	waiting     bool
	ready       bool
}

// Init focuses the textarea and fetches the session's message history from inarid
// so prior conversations are restored when fox reconnects to an existing session.
func (c Chat) Init() tea.Cmd {
	return tea.Batch(c.input.Focus(), fetchChatHistory(c.client, c.sessionID))
}

func (c Chat) SessionID() string   { return c.sessionID }
func (c Chat) SessionName() string { return c.sessionName }

func NewChat(client *ipc.Client, sessionID, sessionName, model string) Chat {
	ta := textarea.New()
	ta.Placeholder = "Message " + sessionName + " (" + model + ")..."
	ta.Focus()
	ta.SetHeight(2)
	ta.CharLimit = 2048

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = thinkingStyle

	return Chat{
		client:      client,
		sessionID:   sessionID,
		sessionName: sessionName,
		model:       model,
		input:       ta,
		spinner:     sp,
	}
}

// viewportContent returns the string to show in the viewport.
// When waiting, the animated spinner line is appended; it is never stored in display
// so that chatReplyMsg can simply append the real reply without any pop logic.
func (c Chat) viewportContent() string {
	base := strings.Join(c.display, "\n")
	if !c.waiting {
		return base
	}
	thinking := thinkingStyle.Render(c.spinner.View() + " thinking…")
	if base == "" {
		return thinking
	}
	return base + "\n" + thinking
}

func fetchChatHistory(client *ipc.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		messages, err := client.History(sessionID)
		return chatHistoryMsg{messages: messages, err: err}
	}
}

func (c Chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case chatHistoryMsg:
		if msg.err != nil || len(msg.messages) == 0 {
			return c, nil
		}
		for _, m := range msg.messages {
			switch m.Role {
			case "user":
				c.display = append(c.display, userStyle.Render("you: ")+m.Content)
			case "assistant":
				c.display = append(c.display, assistantStyle.Render(c.sessionName+": ")+m.Content)
			}
		}
		c.viewport.SetContent(c.viewportContent())
		c.viewport.GotoBottom()
		return c, nil

	case chatReplyMsg:
		c.waiting = false
		if msg.err != nil {
			c.display = append(c.display, errorStyle.Render("error: "+msg.err.Error()))
		} else {
			c.display = append(c.display, assistantStyle.Render(c.sessionName+": ")+msg.text)
		}
		c.viewport.SetContent(c.viewportContent())
		c.viewport.GotoBottom()
		return c, nil

	case spinner.TickMsg:
		if !c.waiting {
			return c, nil
		}
		var cmd tea.Cmd
		c.spinner, cmd = c.spinner.Update(msg)
		c.viewport.SetContent(c.viewportContent())
		c.viewport.GotoBottom()
		return c, cmd

	case tea.WindowSizeMsg:
		// model header(1) + sysbar(1) + chat header(1) + border-top(1) + border-bottom(1) +
		// textarea-border-top(1) + textarea-content(2) + textarea-border-bottom(1) + hint(1) = 10 reserved.
		// The bubbles textarea always renders with a border regardless of focus state.
		// Viewport width shrinks by 2 for the border's left and right columns.
		height := msg.Height - 10
		if height < 1 {
			height = 1
		}
		// Textarea and viewport expand to the terminal width, capped at UIWidth.
		// Subtract 2 for the left+right border columns that each component adds.
		contentW := msg.Width
		if contentW > UIWidth {
			contentW = UIWidth
		}
		c.input.SetWidth(contentW - 2)
		if !c.ready {
			c.viewport = viewport.New(contentW-2, height)
			c.ready = true
		} else {
			c.viewport.Width = contentW - 2
			c.viewport.Height = height
		}
		c.viewport.SetContent(c.viewportContent())
		c.viewport.GotoBottom()
		return c, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && !c.waiting {
			text := strings.TrimSpace(c.input.Value())
			if text == "" {
				return c, nil
			}
			c.display = append(c.display, userStyle.Render("you: ")+text)
			c.input.Reset()
			c.waiting = true
			c.viewport.SetContent(c.viewportContent())
			c.viewport.GotoBottom()
			return c, tea.Batch(sendMessage(c.client, c.sessionID, text), c.spinner.Tick)
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
		H("[ctrl+o] model"),
		HS(),
		H("[↑↓] scroll"),
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
