// chat_keys.go owns keyboard handling for the chat view: the tool-approval
// prompt, ctrl-prefixed shortcuts, input-history navigation, tab completion,
// and message send. it does NOT own the top-level Update dispatch (chat.go) or
// mouse handling (chat_mouse.go).

package views

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/provider"
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
	// mark input as focused on any keypress; the actual Focus() cmd is issued by
	// the caller after this returns.
	if !c.inputFocused {
		c.inputFocused = true
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
		// esc exits the active entry mode: dismiss the tools panel or clear an
		// in-progress slash command, returning to plain chat entry.
		if c.showBuiltin {
			c.showBuiltin = false
			return c, nil, true
		}
		if strings.HasPrefix(c.input.Value(), "/") {
			c.input.Reset()
			return c, nil, true
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
		if strings.HasPrefix(text, "/") {
			c.input.Reset()
			c.status = ""
			m, cmd := c.handleSlashCommand(text)
			return m.(Chat), cmd, true
		}
		if c.offline {
			return c, nil, true
		}
		c.inputHistory = append(c.inputHistory, text)
		c.historyIdx = -1
		c.historyDraft = ""
		c.messages = append(c.messages, provider.Message{Role: "user", Content: text})
		c.display = append(c.display, userStyle.Render("you: ")+text)
		c.ctxChars += len(text)
		c.input.Reset()
		c.status = ""
		c.waiting = true
		c.loadingModel = ""
		// start stream goroutine; store channels on the struct so ChatTokenMsg
		// handlers can schedule the next readNextToken without carrying them in the message.
		tokens := make(chan string, 64)
		statuses := make(chan string, 4)
		errc := make(chan error, 1)
		toolReqs := make(chan ipc.ToolRequestMsg, 1)
		approvals := make(chan bool, 1)
		go func() {
			err := c.client.ChatStream(c.sessionID, text, tokens, statuses, toolReqs, approvals)
			errc <- err
			close(tokens)
			close(statuses)
			close(toolReqs)
		}()
		c.streamTokens = tokens
		c.streamStatus = statuses
		c.streamErrc = errc
		c.toolReqs = toolReqs
		c.toolApprovals = approvals

		setViewportContent(&c.viewport, c.viewportContent())
		c.viewport.GotoBottom()
		return c, tea.Batch(readNextToken(c.sessionID, tokens, statuses, errc, toolReqs), c.spinner.Tick), true
	}
	return c, nil, false
}
