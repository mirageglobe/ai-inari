package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-sudama/internal/ipc"
)

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
)

// Chat is the interactive head-sudama conversation view.
type Chat struct {
	client   *ipc.Client
	viewport viewport.Model
	input    textarea.Model
	history  []string
}

func (c Chat) Init() tea.Cmd { return nil }

func NewChat(client *ipc.Client) Chat {
	ta := textarea.New()
	ta.Placeholder = "Message Head Sudama..."
	ta.Focus()
	ta.SetHeight(3)
	ta.CharLimit = 2048

	vp := viewport.New(80, 16)

	return Chat{
		client:   client,
		viewport: vp,
		input:    ta,
	}
}

func (c Chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlD:
			text := strings.TrimSpace(c.input.Value())
			if text == "" {
				return c, nil
			}
			c.history = append(c.history, userStyle.Render("you: ")+text)
			c.input.Reset()
			c.viewport.SetContent(strings.Join(c.history, "\n"))
			c.viewport.GotoBottom()
			// TODO: send to daemon and stream response back
			return c, nil
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
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("CHAT — Head Sudama")
	hint := lipgloss.NewStyle().Faint(true).Render("ctrl+d send  esc back")
	return header + "\n" + c.viewport.View() + "\n" + c.input.View() + "\n" + hint
}
