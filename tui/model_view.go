// model_view.go owns the root model's View: it selects the body (active view or
// an overlay), prepends the top bar, sets the cursor shape, and pads the frame
// to the terminal height. it does NOT own Update dispatch or message handling.

package tui

import (
	"strings"

	"github.com/mirageglobe/ai-inari/tui/views"
)

func (m Model) View() string {
	topBar := views.RenderTopBar(m.connErr, m.sysStats, m.termWidth, m.titleColorIdx) + "\n"

	var body string
	switch m.topOverlay() {
	case overlayModelSelector:
		body = m.models.RenderModal(m.termWidth, m.termHeight-1)
	case overlayAgents:
		body = m.agents.RenderModal(m.termWidth, m.termHeight-1)
	case overlayDescribe:
		body = m.describe.RenderModal(m.termWidth, m.termHeight-1)
	case overlayLogs:
		body = m.logs.RenderModal(m.termWidth, m.termHeight-1)
	case overlayThemePicker:
		body = views.RenderThemeOverlay(m.themePickerIdx, m.termWidth, m.termHeight-1)
	case overlayHelp:
		// -1 to leave the top bar row; Place fills the remaining rows.
		body = views.RenderHelpOverlay(m.currentViewName(), m.termWidth, m.termHeight-1)
	default:
		switch m.current {
		case viewChat:
			body = m.chats[m.activeSession].View()
		default:
			// suppress the agents table for the single frame between launch and the
			// initial session fetch resolving, so a session with a model already
			// assigned lands straight in chat instead of flashing the table first.
			if !m.agents.Booting() {
				body = m.agents.View()
			}
		}
	}

	// emit cursor shape once here; views no longer emit escape sequences themselves.
	// the agents view has no text input, so it never shows the blinking bar cursor.
	cursorEsc := views.ResetCursor
	overlayOpen := m.topOverlay() != overlayNone
	if !overlayOpen && m.current == viewChat {
		if chat, ok := m.chats[m.activeSession]; ok && chat.InputFocused() {
			cursorEsc = views.BlinkBarCursor
		}
	}
	full := cursorEsc + topBar + body
	// pad every render to termHeight lines so Bubble Tea's cursor tracking stays
	// consistent when switching between views of different heights. Without this,
	// switching from a short view (models, describe) back to a tall one (agents)
	// positions the cursor mid-screen, causing the top lines including the header
	// to render into stale rows and appear invisible.
	if m.termHeight > 0 {
		if pad := m.termHeight - 1 - strings.Count(full, "\n"); pad > 0 {
			full += strings.Repeat("\n", pad)
		}
	}
	return full
}
