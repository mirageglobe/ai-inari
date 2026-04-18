package views

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

// UIWidth is the target maximum width for all fox UI elements.
const UIWidth = 120

var HeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).MaxWidth(UIWidth)
var ConnOKStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
var ConnErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

var (
	hintActiveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintDisabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// HintCmd is a single entry in the command hint bar.
type HintCmd struct {
	Label   string
	Enabled bool
	isSep   bool // renders as a group divider, not a command
}

// H returns an enabled HintCmd.
func H(label string) HintCmd { return HintCmd{Label: label, Enabled: true} }

// HD returns a disabled (dimmed) HintCmd.
func HD(label string) HintCmd { return HintCmd{Label: label, Enabled: false} }

// HS returns a visual group separator rendered as a dimmed "│".
func HS() HintCmd { return HintCmd{isSep: true} }

var hintSepStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

// RenderHint renders a list of commands, dimming unavailable ones and wrapping
// lines that would exceed width. HS() separators are rendered mid-line as "│"
// and skipped when they would fall at the start of a new line.
// Pass 0 to fall back to UIWidth.
func RenderHint(cmds []HintCmd, width int) string {
	if width <= 0 {
		width = UIWidth
	}

	const gap = "  "
	const sepRaw = " │ "

	var lines []string
	lineRaw := ""
	lineParts := []string{}

	flush := func() {
		if len(lineParts) > 0 {
			lines = append(lines, strings.Join(lineParts, gap))
			lineRaw = ""
			lineParts = nil
		}
	}

	for _, c := range cmds {
		if c.isSep {
			// Only render a separator mid-line; skip it at the start to avoid orphaned dividers.
			if lineRaw != "" && len(lineRaw+sepRaw) <= width {
				lineRaw += sepRaw
				lineParts = append(lineParts, hintSepStyle.Render(" │ "))
			}
			continue
		}

		style := hintActiveStyle
		if !c.Enabled {
			style = hintDisabledStyle
		}
		raw := c.Label
		styled := style.Render(raw)

		candidate := lineRaw
		if candidate != "" {
			candidate += gap + raw
		} else {
			candidate = raw
		}

		if len(candidate) > width && lineRaw != "" {
			flush()
			lineRaw = raw
			lineParts = []string{styled}
		} else {
			lineRaw = candidate
			lineParts = append(lineParts, styled)
		}
	}
	flush()
	return strings.Join(lines, "\n")
}

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
