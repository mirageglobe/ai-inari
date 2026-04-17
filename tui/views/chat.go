package views

import (
	"strings"

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
)

type chatReplyMsg struct {
	reply ollama.Message
	err   error
}

// Chat is the interactive conversation view for a selected model.
// messages is the canonical history sent to Ollama on every turn for context.
// display is the rendered version of that history shown in the viewport.
type Chat struct {
	client   *ipc.Client
	model    string
	messages []ollama.Message
	display  []string
	viewport viewport.Model
	input    textarea.Model
	waiting  bool
}

func (c Chat) Init() tea.Cmd { return nil }

func NewChat(client *ipc.Client, model string) Chat {
	ta := textarea.New()
	ta.Placeholder = "Message " + model + "..."
	ta.Focus()
	ta.SetHeight(3)
	ta.CharLimit = 2048

	vp := viewport.New(80, 16)

	return Chat{
		client:   client,
		model:    model,
		viewport: vp,
		input:    ta,
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
			c.messages = append(c.messages, msg.reply)
			c.display = append(c.display, assistantStyle.Render(c.model+": ")+msg.reply.Content)
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
			userMsg := ollama.Message{Role: "user", Content: text}
			c.messages = append(c.messages, userMsg)
			c.display = append(c.display, userStyle.Render("you: ")+text)
			c.display = append(c.display, lipgloss.NewStyle().Faint(true).Render("thinking..."))
			c.viewport.SetContent(strings.Join(c.display, "\n"))
			c.viewport.GotoBottom()
			c.input.Reset()
			c.waiting = true
			return c, sendMessage(c.client, c.model, c.messages)
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
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("CHAT — " + c.model)
	hint := lipgloss.NewStyle().Faint(true).Render("enter send  esc exit chat  ctrl+c quit")
	return header + "\n" + c.viewport.View() + "\n" + c.input.View() + "\n" + hint
}

func sendMessage(client *ipc.Client, model string, messages []ollama.Message) tea.Cmd {
	return func() tea.Msg {
		reply, err := client.Chat(model, messages)
		if err != nil {
			return chatReplyMsg{err: err}
		}
		return chatReplyMsg{reply: ollama.Message{Role: "assistant", Content: reply}}
	}
}
