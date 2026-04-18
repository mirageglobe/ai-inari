package views

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gocpu "github.com/shirou/gopsutil/v3/cpu"
	gomem "github.com/shirou/gopsutil/v3/mem"
)

var sysBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

// SysStatsMsg carries a refreshed snapshot of CPU and memory.
type SysStatsMsg struct {
	CPUPercent float64
	MemUsed    uint64
	MemTotal   uint64
}

// SysStatsTick returns a command that fires SysStatsMsg after 5 seconds, then
// the caller reschedules it to keep the bar live.
func SysStatsTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(_ time.Time) tea.Msg {
		return fetchSysStats()
	})
}

// FetchSysStatsNow returns an immediate one-shot fetch for first-load.
func FetchSysStatsNow() tea.Cmd {
	return func() tea.Msg { return fetchSysStats() }
}

func fetchSysStats() SysStatsMsg {
	pcts, _ := gocpu.Percent(0, false)
	vm, _ := gomem.VirtualMemory()

	var cpu float64
	if len(pcts) > 0 {
		cpu = pcts[0]
	}
	var used, total uint64
	if vm != nil {
		used = vm.Used
		total = vm.Total
	}
	return SysStatsMsg{CPUPercent: cpu, MemUsed: used, MemTotal: total}
}

