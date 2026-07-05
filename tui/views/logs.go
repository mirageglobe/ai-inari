package views

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// logFile is the inari TUI log written by cmd/inari/main.go.
const logFile = "inari.log"

type logContentMsg struct {
	content string
}

// Logs reads and displays inari.log in a scrollable viewport.
type Logs struct {
	viewport viewport.Model
	content  string
	ready    bool
	width    int // terminal width, used for hint rendering
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
		l.width = msg.Width
		// topbar(1) + border-top(1) + border-bottom(1) + hint(1) = 4 reserved
		height := msg.Height - 4
		if height < 1 {
			height = 1
		}
		// subtract 2 for agentsStyle NormalBorder so total width = UIWidth.
		vpWidth := l.width - 2
		if vpWidth < 1 {
			vpWidth = 1
		}
		if !l.ready {
			l.viewport = viewport.New(vpWidth, height)
			l.ready = true
		} else {
			l.viewport.Width = vpWidth
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
	viewLabel := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary).Render("logs")
	hint := viewLabel + "  " + RenderHint([]HintCmd{H("[r] refresh"), H("[esc] back"), HS(), H("[?] help")}, l.width-6)

	var body string
	if !l.ready {
		body = agentsStyle.Render(lipgloss.NewStyle().Faint(true).Render("loading…"))
	} else if strings.TrimSpace(l.content) == "" {
		body = agentsStyle.Render(lipgloss.NewStyle().Faint(true).Render("(no log entries yet)"))
	} else {
		body = agentsStyle.Render(l.viewport.View())
	}

	return body + "\n" + hint
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
