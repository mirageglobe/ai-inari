// Package views — footer.go owns the shared fox-line and footer rendering used by all views.
// it does NOT own per-view hint lists or status logic — those stay in their respective view files.
package views

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// RenderFoxLine builds the status bar common to all views:
//
//	label | name | model | tokens | cwd
func RenderFoxLine(label, name, model, tokens, cwd string) string {
	sepStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary)
	metaStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
	sep := sepStyle.Render(" | ")
	return labelStyle.Render(label) + sep +
		labelStyle.Render(name) + sep +
		metaStyle.Render(model) + sep +
		metaStyle.Render(tokens) + sep +
		metaStyle.Render(cwd)
}

// renderFooter assembles the five-line footer stack shared by all views:
//
//	foxLine
//	cwdLine    (empty string renders as a blank line)
//	statusMsg  (empty string renders as a blank line)
//	inputView
//	hint
func renderFooter(foxLine, cwdLine, statusMsg, inputView, hint string) string {
	return foxLine + "\n" + cwdLine + "\n" + statusMsg + "\n" + inputView + "\n" + hint
}

// renderCWDLine formats the sandboxed working directory line shown in the footer.
func renderCWDLine(cwd string) string {
	if cwd == "" || cwd == "—" {
		return ""
	}
	labelStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
	pathStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary)
	return labelStyle.Render("cwd [dir]") + " " + pathStyle.Render(cwd)
}

// fmtTokens converts a raw character count to a human-readable token estimate (~4 chars/token).
func fmtTokens(chars int) string {
	t := chars / 4
	if t < 1000 {
		return fmt.Sprintf("~%d tokens", t)
	}
	return fmt.Sprintf("~%.1fk tokens", float64(t)/1000)
}
