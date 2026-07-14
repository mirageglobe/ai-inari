// chat_update.go owns the chat view's top-level Update dispatcher: it routes each
// message type to its dedicated handler (chat_stream.go, chat_msgs.go, etc.) and
// runs the shared fall-through that keeps typing and scrolling flowing to the
// textarea/viewport. it does NOT own the Chat struct (chat.go) or the handlers.

package views

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Update is the top-level message dispatcher for the chat view. each message
// type is handled by a dedicated method (see chat_stream.go, chat_msgs.go,
// chat_keys.go, chat_mouse.go); key and mouse handlers may decline a message so
// that normal typing and wheel scrolling fall through to the shared tail below.
func (c Chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ThemeChangedMsg:
		return c.onThemeChanged()
	case ThemeSaveErrMsg:
		return c.onThemeSaveErr(msg)
	case exportChatResultMsg:
		return c.onExportResult(msg)
	case setCwdResultMsg:
		return c.onSetCwd(msg)
	case renameResultMsg:
		return c.onRename(msg)
	case tagResultMsg:
		return c.onTag(msg)
	case setNumCtxResultMsg:
		return c.onSetNumCtx(msg)
	case roleResultMsg:
		return c.onRole(msg)
	case unassignModelResultMsg:
		return c.onUnassign(msg)
	case clearHistoryResultMsg:
		return c.onClear(msg)
	case compactHistoryResultMsg:
		return c.onCompact(msg)
	case chatHistoryMsg:
		return c.onHistory(msg)
	case modelContextMsg:
		c.maxCtx = msg.max
		return c, nil
	case recapMsg:
		// show the recap in the status line when reopening an idle session; skip
		// if empty or a stream/response is already underway so it never clobbers it.
		if msg.text != "" && !c.waiting && c.status == "" {
			c.status = "[recap] " + msg.text
		}
		return c, nil
	case toolApprovalRequestMsg:
		return c.onToolApproval(msg)
	case ChatTokenMsg:
		return c.onToken(msg)
	case ChatStatusMsg:
		return c.onStatus(msg)
	case ChatDoneMsg:
		return c.onDone(msg)
	case spinner.TickMsg:
		return c.onTick(msg)
	case IdleHintTickMsg:
		return c.onIdleHintTick()
	case tea.WindowSizeMsg:
		return c.onWindowSize(msg)
	case tea.KeyMsg:
		// any keypress is activity: reset the idle timer and drop the current hint.
		c.lastActivity = time.Now()
		c.idleHint = ""
		var cmd tea.Cmd
		var handled bool
		c, cmd, handled = c.handleKey(msg)
		if handled {
			return c, cmd
		}
	case tea.MouseMsg:
		var cmd tea.Cmd
		var handled bool
		c, cmd, handled = c.handleMouse(msg)
		if handled {
			return c, cmd
		}
	}

	// fall-through for normal typing and unhandled mouse events: keep the input
	// focused and forward the message to the textarea and viewport.
	var (
		vpCmd    tea.Cmd
		taCmd    tea.Cmd
		focusCmd tea.Cmd
	)
	if c.inputFocused && !c.input.Focused() {
		focusCmd = c.input.Focus()
	}
	c.viewport, vpCmd = c.viewport.Update(msg)
	c.input, taCmd = c.input.Update(msg)
	return c, tea.Batch(vpCmd, taCmd, focusCmd)
}
