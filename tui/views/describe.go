package views

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-sudama/internal/session"
)

// Describe shows full metadata for the selected session.
type Describe struct {
	sess *session.Session
}

func (d Describe) Init() tea.Cmd { return nil }

func NewDescribe() Describe {
	return Describe{}
}

func (d Describe) SetSession(s *session.Session) Describe {
	d.sess = s
	return d
}

func (d Describe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return d, nil
}

func (d Describe) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("DESCRIBE")
	hint := lipgloss.NewStyle().Faint(true).Render("esc back")

	if d.sess == nil {
		return header + "\n\nNo session selected.\n\n" + hint
	}

	style := lipgloss.NewStyle().Padding(0, 1)
	body := style.Render(
		"ID:      " + d.sess.ID + "\n" +
		"Model:   " + d.sess.Model + "\n" +
		"Tier:    " + string(d.sess.Tier) + "\n" +
		"Status:  " + string(d.sess.Status) + "\n" +
		"Created: " + d.sess.CreatedAt.String(),
	)
	return header + "\n" + body + "\n" + hint
}
