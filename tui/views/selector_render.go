// selector_render.go owns ModelSelector's rendering: modal sizing and the
// RenderModal/View methods. it does NOT own the struct/construction
// (selector.go) or Update (selector_update.go).

package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// WithModalDimensions resizes the selector's table to fit the modal box.
// called once when the modal opens; the table keeps these dimensions until next WindowSizeMsg.
func (m ModelSelector) WithModalDimensions() ModelSelector {
	m.table.SetHeight(8)
	m.refreshRows() // size columns to the shared modal width (falls back to ModalInnerW)
	return m
}

// RenderModal renders the selector as a centred overlay for use on top of the agents view.
func (m ModelSelector) RenderModal(termWidth, termHeight int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary)
	secStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary)

	title := titleStyle.Render("model select")
	if m.targetSessionName != "" {
		title += "  " + secStyle.Render("→ "+m.targetSessionName)
	}

	hintCmds := []HintCmd{H("[enter] assign/pull"), H("[d] delete")}
	if m.targetModel != "" {
		hintCmds = append(hintCmds, H("[u] unload"))
	}
	hintCmds = append(hintCmds, H("[q/esc] cancel"))
	hint := RenderHint(hintCmds, modalInnerWidth(m.width))

	var lines []string
	lines = append(lines, title)
	lines = append(lines, m.table.View())
	if m.status != "" {
		line := m.status
		if m.loading {
			line = m.spinner.View() + " " + line
		}
		lines = append(lines, line)
	}
	lines = append(lines, hint)

	boxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ActiveTheme.Primary).
		Padding(0, 1)

	box := boxStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(termWidth, termHeight, lipgloss.Center, lipgloss.Center, box)
}

// View satisfies tea.Model; actual rendering always goes through RenderModal,
// called directly by the root model with live terminal dimensions.
func (m ModelSelector) View() string {
	return m.table.View()
}
