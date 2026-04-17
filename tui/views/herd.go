package views

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-haniwa/internal/ipc"
	"github.com/mirageglobe/ai-haniwa/internal/session"
)

var herdStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type sessionsMsg []*session.Session

// Herd is the default pod-list view (k9s-style table).
type Herd struct {
	client *ipc.Client
	table  table.Model
}

func NewHerd(client *ipc.Client) Herd {
	cols := []table.Column{
		{Title: "ID",      Width: 24},
		{Title: "Model",   Width: 16},
		{Title: "Tier",    Width: 10},
		{Title: "Status",  Width: 12},
		{Title: "Age",     Width: 10},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	return Herd{client: client, table: t}
}

func (h Herd) Init() tea.Cmd {
	return fetchSessions(h.client)
}

func (h Herd) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsMsg:
		rows := make([]table.Row, len(msg))
		for i, s := range msg {
			rows[i] = table.Row{
				s.ID,
				s.Model,
				string(s.Tier),
				string(s.Status),
				fmt.Sprintf("%.0fs", time.Since(s.CreatedAt).Seconds()),
			}
		}
		h.table.SetRows(rows)
		return h, nil
	}
	var cmd tea.Cmd
	h.table, cmd = h.table.Update(msg)
	return h, cmd
}

func (h Herd) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("HERD")
	hint := lipgloss.NewStyle().Faint(true).Render("l logs  d describe  i chat  q quit")
	return fmt.Sprintf("%s\n%s\n%s", header, herdStyle.Render(h.table.View()), hint)
}

func fetchSessions(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Call("session.list", nil)
		if err != nil || resp.Error != nil {
			return sessionsMsg{}
		}
		b, err := json.Marshal(resp.Result)
		if err != nil {
			return sessionsMsg{}
		}
		var sessions []*session.Session
		if err := json.Unmarshal(b, &sessions); err != nil {
			return sessionsMsg{}
		}
		return sessionsMsg(sessions)
	}
}
