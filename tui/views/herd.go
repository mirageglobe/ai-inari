// Package views contains the individual screen views rendered by the fox TUI:
// Herd (session table), Logs (token stream), Describe (session metadata), and Chat (head-inari conversation).
package views

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/ollama"
)



var herdStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

var (
	connOKStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	connErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	modelsStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

type connStatusMsg struct {
	ok  bool
	err error
}

type modelsMsg struct {
	models []ollama.Model
	err    error
}

type runningMsg struct {
	models []ollama.RunningModel
	err    error
}

type unloadMsg struct {
	err error
}

// Herd is the default pod-list view showing models currently loaded in Ollama.
type Herd struct {
	client  *ipc.Client
	table   table.Model
	connErr string
	status  string
}

func NewHerd(client *ipc.Client) Herd {
	cols := []table.Column{
		{Title: "Model",   Width: 30},
		{Title: "VRAM",    Width: 10},
		{Title: "Sleeps",  Width: 12},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	return Herd{client: client, table: t}
}

const runningRefreshInterval = 5 * time.Second

func (h Herd) Init() tea.Cmd {
	return tea.Batch(checkConn(h.client), fetchRunning(h.client))
}

func (h Herd) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case connStatusMsg:
		if msg.ok {
			h.connErr = ""
		} else {
			h.connErr = msg.err.Error()
		}
		return h, nil
	case runningMsg:
		rows := make([]table.Row, len(msg.models))
		for i, m := range msg.models {
			rows[i] = table.Row{
				m.Name,
				formatBytes(m.SizeVRAM),
				formatExpiry(m.ExpiresAt),
			}
		}
		h.table.SetRows(rows)
		return h, tea.Tick(runningRefreshInterval, func(_ time.Time) tea.Msg {
			return fetchRunning(h.client)()
		})
	case unloadMsg:
		if msg.err != nil {
			h.status = connErrStyle.Render("unload failed: " + msg.err.Error())
		} else {
			h.status = ""
		}
		return h, fetchRunning(h.client)
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			h.status = ""
			return h, tea.Batch(checkConn(h.client), fetchRunning(h.client))
		case "c", "enter":
			if row := h.table.SelectedRow(); len(row) > 0 {
				return h, func() tea.Msg { return SelectModelMsg{Name: row[0]} }
			}
		case "x":
			if row := h.table.SelectedRow(); len(row) > 0 {
				model := row[0]
				h.status = modelsStyle.Render("unloading " + model + "...")
				return h, func() tea.Msg {
					return unloadMsg{err: h.client.UnloadModel(model)}
				}
			}
		}
	}
	var cmd tea.Cmd
	h.table, cmd = h.table.Update(msg)
	return h, cmd
}

func (h Herd) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("HERD")

	var connLine string
	if h.connErr != "" {
		connLine = connErrStyle.Render("✗ inarid: " + h.connErr)
	} else {
		connLine = connOKStyle.Render("✓ inarid connected")
	}

	hint := lipgloss.NewStyle().Faint(true).Render("c/enter chat  x unload  r refresh  m models  l logs  d describe  q quit")
	body := herdStyle.Render(h.table.View())
	if h.status != "" {
		body += "\n" + h.status
	}
	return fmt.Sprintf("%s\n%s\n%s\n%s", header, connLine, body, hint)
}

func checkConn(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		if err := client.Ping(); err != nil {
			return connStatusMsg{ok: false, err: err}
		}
		return connStatusMsg{ok: true}
	}
}

func fetchModels(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		names, err := client.ListModels()
		if err != nil {
			return modelsMsg{err: err}
		}
		return modelsMsg{models: names}
	}
}

func fetchRunning(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		models, err := client.ListRunning()
		if err != nil {
			return runningMsg{err: err}
		}
		return runningMsg{models: models}
	}
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(b)/float64(1<<20))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func formatExpiry(expiresAt string) string {
	t, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return "—"
	}
	d := time.Until(t).Round(time.Second)
	if d <= 0 {
		return "expired"
	}
	return fmt.Sprintf("in %s", d)
}
