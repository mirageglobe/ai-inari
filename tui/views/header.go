package views

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

var HeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
var ConnOKStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
var ConnErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

// ConnStatusMsg is broadcast whenever the connection to inarid is checked.
type ConnStatusMsg struct {
	OK  bool
	Err error
}

// CheckConnNow issues an immediate one-shot ping and returns a ConnStatusMsg.
func CheckConnNow(client *ipc.Client) tea.Cmd {
	return func() tea.Msg { return pingMsg(client) }
}

// ConnTick returns a command that fires a ConnStatusMsg after 3 seconds,
// allowing the caller to reschedule it on receipt to keep the header live.
func ConnTick(client *ipc.Client) tea.Cmd {
	return tea.Tick(3*time.Second, func(_ time.Time) tea.Msg {
		return pingMsg(client)
	})
}

func pingMsg(client *ipc.Client) ConnStatusMsg {
	if err := client.Ping(); err != nil {
		return ConnStatusMsg{OK: false, Err: err}
	}
	return ConnStatusMsg{OK: true}
}

func RenderHeader(connErr string) string {
	var connLine string
	if connErr != "" {
		connLine = ConnErrStyle.Render("○ offline")
	} else {
		connLine = ConnOKStyle.Render("● connected to inari ai daemon")
	}
	return HeaderStyle.Render("🦊 inari fox") + "  " + connLine
}
