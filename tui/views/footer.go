// Package views — footer.go owns the shared session-line and footer rendering used by all views.
// it does NOT own per-view hint lists or status logic — those stay in their respective view files.
package views

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// RenderSessionLine builds the status bar common to all views:
//
//	label | name | model | tokens
func RenderSessionLine(label, name, model, tokens string) string {
	sepStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary)
	metaStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
	sep := sepStyle.Render(" | ")
	return labelStyle.Render(label) + sep +
		labelStyle.Render(name) + sep +
		metaStyle.Render(model) + sep +
		metaStyle.Render(tokens)
}

// renderFooter assembles the five-line footer stack shared by all views:
//
//	sessionLine
//	cwdLine    (empty string renders as a blank line)
//	statusLine  (empty string renders as a blank line)
//	inputLine
//	hintLine
func renderFooter(sessionLine, cwdLine, statusLine, inputLine, hintLine string) string {
	return sessionLine + "\n" + cwdLine + "\n" + statusLine + "\n" + inputLine + "\n" + hintLine
}

// renderCWDLine formats the sandboxed working directory line shown in the footer.
func renderCWDLine(cwd string) string {
	if cwd == "" || cwd == "—" {
		return ""
	}
	labelStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
	pathStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary)
	return labelStyle.Render("[cwd]") + " " + pathStyle.Render(cwd)
}

// fmtTokens converts a raw character count to a human-readable token estimate (~4 chars/token).
func fmtTokens(chars int) string {
	t := chars / 4
	if t < 1000 {
		return fmt.Sprintf("~%d tokens", t)
	}
	return fmt.Sprintf("~%.1fk tokens", float64(t)/1000)
}
