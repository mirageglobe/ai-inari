// Package views — top bar rendering: title animation and connection/system status.
// this file owns the wave title sweep, connection health commands, and RenderTopBar.
// hint bar primitives (HintCmd, RenderHint) live in hints.go.
package views

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

var HeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))

// titleBase is the resting colour of the title text; reassigned by ApplyTheme.
var titleBase = lipgloss.Color("99")

// rayColors defines the highlight falloff around the ray centre.
// characters outside this range render at titleBase.
var rayColors = []struct {
	delta int
	color lipgloss.Color
}{
	{-2, "105"}, // faint edge
	{-1, "141"}, // soft glow
	{0, "147"},  // brightest centre
	{1, "141"},  // soft glow
	{2, "105"},  // faint edge
}

// TitleText is the rendered title string; its rune count drives the sweep length.
const TitleText = "🦊 inari ui │ github.com/mirageglobe/ai-inari"

// TitleLen is the number of runes in TitleText, used by the root model to detect
// when the ray has crossed the full title and the 30s pause should begin.
var TitleLen = len([]rune(TitleText))

// TitleTickMsg advances the ray one character during an active sweep.
type TitleTickMsg struct{}

// TitleStartMsg fires after the 30s pause and begins the next sweep.
type TitleStartMsg struct{}

// TitleTick fires TitleTickMsg after 60ms — one step per character during a sweep.
func TitleTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(_ time.Time) tea.Msg {
		return TitleTickMsg{}
	})
}

// TitlePause fires TitleStartMsg after a random interval between 10 and 30 seconds.
func TitlePause() tea.Cmd {
	d := time.Duration(10+rand.Intn(21)) * time.Second
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return TitleStartMsg{}
	})
}

// renderWaveTitle renders TitleText in titleBase with a bright ray at position offset.
// when offset is negative the ray is off-screen and all characters render at titleBase,
// giving a clean resting state between sweeps.
func renderWaveTitle(offset int) string {
	runes := []rune(TitleText)
	var sb strings.Builder
	for i, r := range runes {
		delta := i - offset
		color := titleBase
		for _, ray := range rayColors {
			if delta == ray.delta {
				color = ray.color
				break
			}
		}
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(color).Render(string(r)))
	}
	return sb.String()
}

// ConnStatusMsg is broadcast whenever the connection to inarid is checked.
type ConnStatusMsg struct {
	OK  bool
	Err error
}

var (
	ConnOKStyle  lipgloss.Style
	ConnErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

// CheckConnNow issues an immediate one-shot ping and returns a ConnStatusMsg.
func CheckConnNow(client *ipc.Client) tea.Cmd {
	return func() tea.Msg { return pingMsg(client) }
}

// ConnTick returns a command that fires a ConnStatusMsg after 1 second,
// allowing the caller to reschedule it on receipt to keep the header live.
// 1 second gives a fast offline→online detection without hammering the socket.
func ConnTick(client *ipc.Client) tea.Cmd {
	return tea.Tick(1*time.Second, func(_ time.Time) tea.Msg {
		return pingMsg(client)
	})
}

func pingMsg(client *ipc.Client) ConnStatusMsg {
	if err := client.Ping(); err != nil {
		return ConnStatusMsg{OK: false, Err: err}
	}
	return ConnStatusMsg{OK: true}
}

// RenderTopBar renders the single top bar: app title left, cpu/mem/connection right-aligned.
// colorIdx is the current wave offset; each character of the title samples a different
// step from wavePalette so the gradient drifts across the text as the offset advances.
func RenderTopBar(connErr string, stats SysStatsMsg, width, colorIdx int) string {
	if width <= 0 {
		width = UIWidth
	}
	left := renderWaveTitle(colorIdx)

	cpu := fmt.Sprintf("cpu %.0f%%", stats.CPUPercent)
	mem := fmt.Sprintf("mem %s / %s", formatBytes(int64(stats.MemUsed)), formatBytes(int64(stats.MemTotal)))
	sysText := sysBarStyle.Render(cpu + "  " + mem)

	var connText string
	if connErr != "" {
		connText = ConnErrStyle.Render("  ○ 👹 inarid offline")
	} else {
		connText = ConnOKStyle.Render("  ● 👹 inarid online")
	}

	right := sysText + connText
	pad := width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}
