// describe_render.go owns the Describe view's content building and View
// rendering. it does NOT own the struct/construction (describe.go) or
// Update (describe_update.go).

package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (d Describe) buildContent() string {
	if d.sessID == "" {
		return "no session selected."
	}

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Width(12)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	faintStyle := lipgloss.NewStyle().Faint(true)

	row := func(label, value string) string {
		return labelStyle.Render(label) + valueStyle.Render(value)
	}

	msgCountStr := fmt.Sprintf("%d messages", d.msgCount)
	if !d.fetched {
		msgCountStr = "fetching…"
	}

	vramStr := "—"
	if d.vram > 0 {
		vramStr = formatBytes(d.vram)
	}

	modelStr := d.model
	if modelStr == "" {
		modelStr = "—"
	}

	var behaviorBlock string
	if d.systemPrompt == "" {
		behaviorBlock = labelStyle.Render("behavior") + "\n" + faintStyle.Render("— not set  ([e] to edit behavior)")
	} else {
		behaviorBlock = labelStyle.Render("behavior") + "\n" + valueStyle.Render(d.systemPrompt)
	}

	return row("name", d.sessName) + "\n" +
		row("id", d.sessID) + "\n" +
		row("model", modelStr) + "\n" +
		row("vram", vramStr) + "\n" +
		row("history", msgCountStr) + "\n" +
		behaviorBlock
}

// RenderModal renders describe as a centred popup modal over the current view
// (chat or agents). the metadata/editor content matches View, wrapped in a
// bordered box. when not editing, the hint advertises the q/esc close; while
// editing, ctrl+s saves and esc exits edit mode (handled in describe_update).
func (d Describe) RenderModal(termWidth, termHeight int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary)

	var lines []string
	if d.editing {
		editLabel := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render("editing behavior")
		hintCmds := []HintCmd{H("[ctrl+s] save"), H("[esc] done")}
		if d.saving {
			hintCmds[0] = HD("[ctrl+s] saving…")
		}
		if d.saveErr != "" {
			hintCmds = append(hintCmds, HS(), HD(d.saveErr))
		}
		lines = []string{titleStyle.Render("describe") + "  " + editLabel, d.input.View(), RenderHint(hintCmds, modalInnerWidth(d.width))}
	} else {
		editHint := H("[e] edit behavior")
		if d.offline {
			editHint = HD("[e] edit behavior")
		}
		hint := RenderHint([]HintCmd{editHint, HS(), H("[q/esc] close")}, modalInnerWidth(d.width))
		lines = []string{titleStyle.Render("describe"), d.buildContent(), hint}
	}

	boxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ActiveTheme.Primary).
		Padding(0, 1)
	box := boxStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(termWidth, termHeight, lipgloss.Center, lipgloss.Center, box)
}

func (d Describe) View() string {
	viewLabel := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary).Render("describe")
	labelW := 10 // len("describe") + len("  ")

	if d.editing {
		editLabel := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render("  editing behavior")
		var hintCmds []HintCmd
		if d.saving || d.offline {
			hintCmds = []HintCmd{HD("[ctrl+s] save"), HD("[esc] cancel")}
			if d.saving {
				hintCmds[0] = HD("[ctrl+s] saving…")
			}
		} else {
			hintCmds = []HintCmd{H("[ctrl+s] save"), H("[esc] cancel")}
		}
		if d.saveErr != "" {
			hintCmds = append(hintCmds, HS(), HD(d.saveErr))
		}
		hint := viewLabel + editLabel + "  " + RenderHint(hintCmds, d.width-labelW-len("  editing behavior")-2)
		return d.input.View() + "\n" + hint
	}

	editHint := H("[e] edit behavior")
	if d.offline {
		editHint = HD("[e] edit behavior")
	}
	hint := viewLabel + "  " + RenderHint([]HintCmd{editHint, H("[esc] back"), HS(), H("[?] help")}, d.width-labelW)

	var body string
	if !d.ready {
		body = agentsStyle.Render(lipgloss.NewStyle().Faint(true).Render("loading…"))
	} else {
		body = agentsStyle.Render(d.viewport.View())
	}

	return body + "\n" + hint
}
