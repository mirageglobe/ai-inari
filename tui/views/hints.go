// Package views — hint bar rendering shared across all views.
// this file owns the HintCmd type, constructor helpers, RenderHint, and RenderScrollbar.
// it does NOT own view-specific hint lists — those live in their respective view files.
package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// UIWidth is the fallback terminal width used before the first WindowSizeMsg arrives.
const UIWidth = 100

var (
	hintLabelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	hintActiveStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintDisabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	hintSepStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
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

// RenderHint renders a list of commands, dimming unavailable ones and wrapping
// lines that would exceed width. HS() separators are rendered mid-line as "│"
// and skipped when they would fall at the start of a new line.
// pass 0 to fall back to UIWidth.
func RenderHint(cmds []HintCmd, width int) string {
	if width <= 0 {
		width = UIWidth
	}

	const gap = "  "
	const sepRaw = " │ "
	const prefixRaw = "hint: "
	prefix := hintLabelStyle.Render(prefixRaw)

	var lines []string
	lineRaw := prefixRaw
	lineParts := []string{prefix}

	flush := func() {
		if len(lineParts) > 0 {
			lines = append(lines, strings.Join(lineParts, gap))
			lineRaw = ""
			lineParts = nil
		}
	}

	for _, c := range cmds {
		if c.isSep {
			// only render a separator mid-line; skip it at the start to avoid orphaned dividers.
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

// RenderScrollbar returns a 1-character-wide vertical scrollbar for the given viewport.
// returns an empty string when all content fits without scrolling.
func RenderScrollbar(vp viewport.Model) string {
	total := vp.TotalLineCount()
	h := vp.Height
	if total <= h || h <= 0 {
		return ""
	}
	thumbH := max(1, h*h/total)
	maxOffset := total - h
	thumbTop := 0
	if maxOffset > 0 {
		thumbTop = (h - thumbH) * vp.YOffset / maxOffset
	}
	trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	thumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	var sb strings.Builder
	for i := range h {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if i >= thumbTop && i < thumbTop+thumbH {
			sb.WriteString(thumbStyle.Render("█"))
		} else {
			sb.WriteString(trackStyle.Render("░"))
		}
	}
	return sb.String()
}
