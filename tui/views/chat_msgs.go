// chat_msgs.go owns the chat view's non-streaming result handlers: theme
// changes, export/unassign/clear results, history load, and window resize.
// it does NOT own streaming handlers (chat_stream.go) or the top-level Update
// dispatch (chat.go).

package views

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// onThemeChanged restyles the spinner and rebuilds the rendered display so the
// new theme colours take effect.
func (c Chat) onThemeChanged() (tea.Model, tea.Cmd) {
	c.spinner.Style = spinnerStyle
	c.rebuildDisplay()
	return c, nil
}

// onThemeSaveErr surfaces a failed theme write in the status line.
func (c Chat) onThemeSaveErr(msg ThemeSaveErrMsg) (tea.Model, tea.Cmd) {
	c.status = "[warn] theme save failed: " + msg.Err.Error()
	return c, nil
}

// onExportResult reports where the session context was written, or why it failed.
func (c Chat) onExportResult(msg exportChatResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] save failed: " + msg.err.Error()
	} else {
		c.status = "[saved] " + msg.path
	}
	return c, nil
}

// onUnassign clears the displayed model after a successful model unload.
func (c Chat) onUnassign(msg unassignModelResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] unassign failed: " + msg.err.Error()
		return c, nil
	}
	c = c.WithModel("")
	c.status = "[info] model unloaded"
	return c, nil
}

// onClear empties the conversation after a successful /clear.
func (c Chat) onClear(msg clearHistoryResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] clear failed: " + msg.err.Error()
		return c, nil
	}
	c.messages = nil
	c.ctxChars = 0
	c.historyLoaded = true
	c.rebuildDisplay()
	c.status = ""
	return c, nil
}

// onHistory loads prior session messages once, restoring a conversation on
// reconnect.
func (c Chat) onHistory(msg chatHistoryMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || c.historyLoaded {
		return c, nil
	}
	// mark loaded even when messages is empty; a new session has no history yet,
	// but historyLoaded must be true so a later Init() (e.g. after a model change)
	// does not re-append the now-populated history on top of what is already shown.
	c.historyLoaded = true
	if len(msg.messages) > 0 {
		c.messages = append(c.messages, msg.messages...)
	}
	// rebuild unconditionally so the pre-context line still renders for a new session.
	c.rebuildDisplay()
	return c, nil
}

// onWindowSize recomputes the viewport and input dimensions on a terminal resize.
func (c Chat) onWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	// topbar(1) + border-top(1) + viewport(h) + border-bottom(1) +
	// textarea(1) + sessionLine(1) + cwdLine(1) + statusLine(1) + hint(1) = h+8 total.
	height := msg.Height - 8
	if height < 1 {
		height = 1
	}
	// textarea and viewport expand to the full terminal width.
	// subtract 2 for the left+right border columns that each component adds.
	contentW := msg.Width
	c.input.SetWidth(contentW - 2)
	// viewport fills the border interior; the right border is rendered separately by RenderRightEdge.
	if !c.ready {
		c.viewport = viewport.New(contentW-2, height)
		c.viewport.KeyMap = arrowOnlyKeyMap()
		c.ready = true
	} else {
		c.viewport.Width = contentW - 2
		c.viewport.Height = height
	}
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
	return c, nil
}
