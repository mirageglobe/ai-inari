// hint bar rendering shared across all views.
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
	const prefixRaw = "[hint] "
	prefix := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true).Render(prefixRaw)

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

// RenderRightEdge returns a 1-character-wide right border column for the chat box.
// height covers the full box: top corner + viewport rows + bottom corner.
// when content overflows, thumb rows use ┃ (thick) instead of │ to indicate scroll position.
func RenderRightEdge(vp viewport.Model) string {
	h := vp.Height
	if h <= 0 {
		return ""
	}
	borderStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Primary)
	thumbStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Primary).Bold(true)

	total := vp.TotalLineCount()
	scrollable := total > h
	thumbTop, thumbH := 0, 0
	if scrollable {
		thumbH = max(1, h*h/total)
		maxOffset := total - h
		if maxOffset > 0 {
			thumbTop = (h - thumbH) * vp.YOffset / maxOffset
		}
	}

	var sb strings.Builder
	sb.WriteString(borderStyle.Render("┐"))
	for i := range h {
		sb.WriteByte('\n')
		if scrollable && i >= thumbTop && i < thumbTop+thumbH {
			sb.WriteString(thumbStyle.Render("┃"))
		} else {
			sb.WriteString(borderStyle.Render("│"))
		}
	}
	sb.WriteByte('\n')
	sb.WriteString(borderStyle.Render("┘"))
	return sb.String()
}
