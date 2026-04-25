// Package views — theme definitions and active-theme management.
// this file owns the Theme type, the built-in palette list, and ApplyTheme which
// reassigns all package-level style vars so subsequent renders pick up the new colours.
// it does NOT own config persistence — the root model calls Save after cycling.
package views

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// ThemeChangedMsg is emitted when the active theme is updated.
type ThemeChangedMsg struct{}

// ApplyTableStyles applies the current theme's selection style to the given table.
// call this when initialising a table and when the theme changes.
func ApplyTableStyles(t *table.Model) {
	s := table.DefaultStyles()
	s.Selected = s.Selected.Foreground(ActiveTheme.Secondary).Bold(true)
	t.SetStyles(s)
}

// Theme holds the colour palette for a named scheme.
// Primary is used for headers, borders, and accent text.
// Secondary is used for highlights, spinners, and key labels.
// User is the colour for user-authored chat messages.
// Ray is the five-step gradient used by the title sweep animation (faint → bright → faint).
type Theme struct {
	Name      string
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	User      lipgloss.Color
	Ray       [5]lipgloss.Color
}

// Themes is the ordered list of built-in colour schemes.
// the first entry is the default; [t] cycles forward through this slice.
var Themes = []Theme{
	{
		Name: "purple", Primary: "99", Secondary: "214", User: "212",
		Ray: [5]lipgloss.Color{"105", "141", "147", "141", "105"},
	},
	{
		Name: "amber", Primary: "214", Secondary: "172", User: "220",
		Ray: [5]lipgloss.Color{"136", "178", "214", "178", "136"},
	},
	{
		Name: "slate", Primary: "111", Secondary: "153", User: "159",
		Ray: [5]lipgloss.Color{"67", "75", "111", "75", "67"},
	},
	{
		Name: "rose", Primary: "211", Secondary: "217", User: "225",
		Ray: [5]lipgloss.Color{"168", "175", "211", "175", "168"},
	},
}

// ActiveTheme holds the currently applied palette.
// view files read ActiveTheme.Primary/Secondary/User for inline renders.
var ActiveTheme = Themes[0]

// ThemeIndex returns the index of the named theme, or 0 if not found.
func ThemeIndex(name string) int {
	for i, t := range Themes {
		if t.Name == name {
			return i
		}
	}
	return 0
}

// ApplyTheme updates ActiveTheme and all package-level style variables.
// must be called from the Bubble Tea main goroutine to avoid data races.
func ApplyTheme(t Theme) {
	ActiveTheme = t

	// header / title animation
	HeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
	titleBase = t.Primary
	rayColors = []struct {
		delta int
		color lipgloss.Color
	}{
		{-2, t.Ray[0]},
		{-1, t.Ray[1]},
		{0, t.Ray[2]},
		{1, t.Ray[3]},
		{2, t.Ray[4]},
	}

	// chat
	userStyle      = lipgloss.NewStyle().Foreground(t.User).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(t.Primary)

	// herd / shared pane border (used by herd, chat, logs, describe, selector)
	herdStyle    = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(t.Primary)
	spinnerStyle = lipgloss.NewStyle().Foreground(t.Secondary)

	// hints — active labels use secondary so they're distinct from header primary
	hintActiveStyle = lipgloss.NewStyle().Foreground(t.Secondary)

	// help overlay
	helpTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
	helpKeyStyle   = lipgloss.NewStyle().Foreground(t.Secondary).Bold(true)
	helpBoxStyle   = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(t.Primary).
			Padding(1, 3)
}
