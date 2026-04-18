package views

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// logFile is the fox UI log written by cmd/fox/main.go.
const logFile = "fox.log"

type logContentMsg struct {
	content string
}

// Logs reads and displays fox.log in a scrollable viewport.
type Logs struct {
	viewport viewport.Model
	content  string
	ready    bool
}

func NewLogs() Logs {
	return Logs{}
}

func (l Logs) Init() tea.Cmd { return readLogFile() }

func (l Logs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case logContentMsg:
		l.content = msg.content
		if l.ready {
			l.viewport.SetContent(l.content)
			l.viewport.GotoBottom()
		}
		return l, nil

	case tea.WindowSizeMsg:
		headerHeight := 2 // header + newline
		footerHeight := 2 // hint + newline
		height := msg.Height - headerHeight - footerHeight
		if height < 1 {
			height = 1
		}
		if !l.ready {
			l.viewport = viewport.New(msg.Width, height)
			l.ready = true
		} else {
			l.viewport.Width = msg.Width
			l.viewport.Height = height
		}
		l.viewport.SetContent(l.content)
		l.viewport.GotoBottom()
		return l, nil

	case tea.KeyMsg:
		if msg.String() == "r" {
			return l, readLogFile()
		}
	}

	if l.ready {
		var cmd tea.Cmd
		l.viewport, cmd = l.viewport.Update(msg)
		return l, cmd
	}
	return l, nil
}

func (l Logs) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("LOGS") +
		"  " + lipgloss.NewStyle().Faint(true).Render(logFile)
	hint := RenderHint([]HintCmd{H("[r] refresh"), H("[esc] back")}, l.viewport.Width)

	var body string
	if !l.ready {
		body = lipgloss.NewStyle().Faint(true).Render("loading…")
	} else if strings.TrimSpace(l.content) == "" {
		body = lipgloss.NewStyle().Faint(true).Render("(no log entries yet)")
	} else {
		body = l.viewport.View()
	}

	return header + "\n" + body + "\n" + hint
}

func readLogFile() tea.Cmd {
	return func() tea.Msg {
		b, err := os.ReadFile(logFile)
		if err != nil {
			return logContentMsg{content: ""}
		}
		return logContentMsg{content: string(b)}
	}
}
