// chat_keys.go owns keyboard handling for the chat view: the tool-approval
// prompt, ctrl-prefixed shortcuts, input-history navigation, and tab completion.
// on [enter] it delegates the actual send to chat_send.go. it does NOT own the
// top-level Update dispatch (chat_update.go) or mouse handling (chat_mouse.go).

package views

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleKey processes a key message. it returns the updated chat, an optional
// command, and whether the key was fully handled; when handled is false the
// caller falls through to the shared textarea/viewport update so normal typing
// reaches the input.
func (c Chat) handleKey(msg tea.KeyMsg) (Chat, tea.Cmd, bool) {
	// tool approval prompt intercepts all keys while a tool request is pending.
	if c.pendingTool != nil {
		switch msg.String() {
		case "y", "Y":
			c.runningTool = c.pendingTool.Name
			toolLine := thinkingStyle.Render("[ tool ] "+c.pendingTool.Name) + "  " + thinkingStyle.Render(formatToolArgs(c.pendingTool.Args))
			c.display = append(c.display, toolLine)
			c.toolApprovals <- true
			c.pendingTool = nil
			c.waiting = true
			setViewportContent(&c.viewport, c.viewportContent())
			c.viewport.GotoBottom()
			return c, tea.Batch(readNextToken(c.sessionID, c.streamTokens, c.streamStatus, c.streamErrc, c.toolReqs), c.spinner.Tick), true
		case "n", "N", "esc":
			c.toolApprovals <- false
			c.pendingTool = nil
			c.waiting = true
			setViewportContent(&c.viewport, c.viewportContent())
			c.viewport.GotoBottom()
			return c, tea.Batch(readNextToken(c.sessionID, c.streamTokens, c.streamStatus, c.streamErrc, c.toolReqs), c.spinner.Tick), true
		}
		return c, nil, true // absorb other keys while approval is pending
	}
	// the tools modal captures q and esc to close: it overlays the chat input, so a
	// plain q would otherwise type into the message box. must sit before the input
	// focus mark and the ctrl switch so both keys reliably dismiss the modal.
	if c.showBuiltin && (msg.String() == "q" || msg.String() == "esc") {
		c.showBuiltin = false
		return c, nil, true
	}
	// mark input as focused on any keypress; the actual Focus() cmd is issued by
	// the caller after this returns.
	if !c.inputFocused {
		c.inputFocused = true
	}

	// `!` is a mode, not a prefix character. typing it at an empty input enters shell
	// mode and is consumed, so it never lands in the buffer; mid-line it is just a
	// character (echo hi!) and falls through. backspace at an empty input is the only
	// way out, matching how claude code behaves. the guard on a non-empty buffer is
	// what stops a typo mid-command from silently dropping the mode.
	if !c.shellMode && msg.String() == "!" && c.input.Value() == "" {
		c.shellMode = true
		return c, nil, true
	}
	if c.shellMode && msg.Type == tea.KeyBackspace && c.input.Value() == "" {
		c.shellMode = false
		return c, nil, true
	}

	// ctrl-prefixed shortcuts never collide with typed text, so they stay active
	// while the input is focused (unlike bare keys like `t`/`?`; see open issue).
	// ctrl+m is deliberately omitted: terminals deliver it as carriage-return, which
	// would shadow [enter] send. tools and help are reached via /tools and /help
	// instead of ctrl-prefixed bindings, since both are slash commands anyway.
	switch msg.String() {
	case "ctrl+p":
		// open the slash command palette by seeding the input with "/".
		c.input.SetValue("/")
		c.input.CursorEnd()
		return c, nil, true
	case "esc":
		// esc clears an in-progress slash command, returning to plain chat entry.
		// (the tools modal's own q/esc close is handled earlier.)
		if strings.HasPrefix(c.input.Value(), "/") {
			c.input.Reset()
			return c, nil, true
		}
		// otherwise, esc interrupts an in-flight response (waiting or mid-stream);
		// the daemon cancels generation and the stream ends via its normal done path.
		if c.waiting || c.streamBuf != "" {
			c.status = "[info] interrupting..."
			return c, interruptStream(c.client, c.sessionID), true
		}
	}
	// up/down navigate input history instead of scrolling the viewport.
	if msg.String() == "up" && !c.waiting {
		if len(c.inputHistory) == 0 {
			return c, nil, true
		}
		if c.historyIdx == -1 {
			c.historyDraft = c.input.Value()
			c.historyIdx = len(c.inputHistory) - 1
		} else if c.historyIdx > 0 {
			c.historyIdx--
		}
		c.input.SetValue(c.inputHistory[c.historyIdx])
		c.input.CursorEnd()
		return c, nil, true
	}
	if msg.String() == "down" && !c.waiting {
		if c.historyIdx == -1 {
			return c, nil, true
		}
		if c.historyIdx < len(c.inputHistory)-1 {
			c.historyIdx++
			c.input.SetValue(c.inputHistory[c.historyIdx])
		} else {
			c.historyIdx = -1
			c.input.SetValue(c.historyDraft)
		}
		c.input.CursorEnd()
		return c, nil, true
	}
	if msg.Type == tea.KeyTab {
		inputVal := c.input.Value()
		if strings.HasPrefix(inputVal, "/") {
			for _, cmd := range chatCommandTable {
				if strings.HasPrefix(cmd.Name, inputVal) {
					c.input.SetValue(cmd.Name)
					c.input.CursorEnd()
					return c, nil, true
				}
			}
		}
		return c, nil, true
	}
	if msg.Type == tea.KeyEnter && !c.waiting {
		text := strings.TrimSpace(c.input.Value())
		if text == "" {
			return c, nil, true
		}
		// shell mode is checked before slash commands so the prompt never lies: while
		// [sh] is showing, the whole line goes to the shell. leave the mode with
		// backspace at an empty input to reach slash commands again.
		//
		// shell escape-hatch: run the line as a real shell command via the daemon,
		// bypassing the model. runs even while offline (shell exec is local to the
		// daemon, independent of the model backend). the `!` prefix still works for a
		// pasted line, which never enters the mode since it is not a lone keypress at
		// an empty input; the mode itself survives the Reset below, which is what lets
		// consecutive commands skip the prefix.
		if c.shellMode || strings.HasPrefix(text, "!") {
			line := strings.TrimSpace(strings.TrimPrefix(text, "!"))
			c.input.Reset()
			c.status = ""
			if line == "" {
				return c, nil, true
			}
			nc, cmd := c.runShell(line)
			return nc, cmd, true
		}
		if strings.HasPrefix(text, "/") {
			c.input.Reset()
			c.status = ""
			m, cmd := c.handleSlashCommand(text)
			return m.(Chat), cmd, true
		}
		if c.offline {
			return c, nil, true
		}
		// hand off to sendChat, which records the message and starts the stream.
		c, cmd := c.sendChat(text)
		return c, cmd, true
	}
	return c, nil, false
}
