// chat_mouse.go owns mouse handling for the chat view: click-to-focus, drag
// selection within the viewport, and copy-on-release. it does NOT own the
// top-level Update dispatch (chat.go) or key handling (chat_keys.go).

package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleMouse processes a mouse message. it returns the updated chat, an
// optional command, and whether the event was fully handled; when handled is
// false (e.g. wheel scroll) the caller forwards the event to the viewport.
func (c Chat) handleMouse(msg tea.MouseMsg) (Chat, tea.Cmd, bool) {
	// topbar(1) + border-top(1) = viewport content starts at terminal row 2.
	const viewportTopY = 2
	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonLeft {
			viewportLastY := viewportTopY + c.viewport.Height - 1
			if msg.Y >= viewportTopY && msg.Y <= viewportLastY {
				// click inside viewport: start selection, blur input
				cl := c.viewport.YOffset + (msg.Y - viewportTopY)
				c.selActive = true
				c.selStartLine = cl
				c.selEndLine = cl
				setViewportContentWithSel(&c.viewport, c.viewportContent(), c.selStartLine, c.selEndLine)
				if c.inputFocused {
					c.inputFocused = false
					c.input.Blur()
				}
			} else if msg.Y > viewportLastY && !c.inputFocused {
				// click in footer: focus input
				c.inputFocused = true
				return c, c.input.Focus(), true
			}
			return c, nil, true
		}
	case tea.MouseActionMotion:
		if c.selActive {
			cl := c.viewport.YOffset + (msg.Y - viewportTopY)
			c.selEndLine = cl
			setViewportContentWithSel(&c.viewport, c.viewportContent(), c.selStartLine, c.selEndLine)
			return c, nil, true
		}
	case tea.MouseActionRelease:
		if c.selActive {
			cl := c.viewport.YOffset + (msg.Y - viewportTopY)
			c.selEndLine = cl
			if text := c.selectedText(); text != "" {
				// surface clipboard failures (e.g. pbcopy/xclip absent) rather than
				// reporting a copy that silently did nothing.
				if err := copyToClipboard(text); err != nil {
					c.status = "[warn] copy failed: " + err.Error()
				} else {
					n := strings.Count(text, "\n") + 1
					c.status = fmt.Sprintf("[copied] %d lines", n)
				}
			}
			c.selActive = false
			setViewportContent(&c.viewport, c.viewportContent())
			return c, nil, true
		}
	}
	// unhandled mouse events (wheel scroll etc.) fall through to viewport.Update.
	return c, nil, false
}
