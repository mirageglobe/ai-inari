// chat_stream.go owns the chat view's streaming and tool-call message handlers:
// token arrival, stream completion, tool-approval requests, spinner ticks, and
// compaction results. it does NOT own the top-level Update dispatch (chat.go).

package views

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// onToolApproval shows the approval prompt for a server-requested tool call and
// pauses the stream until the user responds.
func (c Chat) onToolApproval(msg toolApprovalRequestMsg) (tea.Model, tea.Cmd) {
	if msg.SessionID != c.sessionID {
		return c, nil
	}
	c.waiting = false
	c.runningTool = ""
	c.pendingTool = &msg
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
	return c, nil
}

// onToken appends a streamed token to the in-progress buffer and schedules the
// next read.
func (c Chat) onToken(msg ChatTokenMsg) (tea.Model, tea.Cmd) {
	if msg.SessionID != c.sessionID {
		return c, nil
	}
	c.streamBuf += msg.Token
	c.waiting = false // hide spinner once first token arrives
	c.runningTool = ""
	c.loadingModel = ""
	c.lastActivity = time.Now() // a streamed token is activity: hold off idle hints
	c.idleHint = ""
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
	return c, readNextToken(c.sessionID, c.streamTokens, c.streamStatus, c.streamErrc, c.toolReqs)
}

// onStatus updates the spinner label to reflect the phase inarid reports:
// "loading" while the assigned model is being cold-loaded into backend memory,
// "thinking" once generation has actually begun.
func (c Chat) onStatus(msg ChatStatusMsg) (tea.Model, tea.Cmd) {
	if msg.SessionID != c.sessionID {
		return c, nil
	}
	if msg.Status == "loading" {
		c.loadingModel = c.model
	} else {
		c.loadingModel = ""
	}
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
	return c, readNextToken(c.sessionID, c.streamTokens, c.streamStatus, c.streamErrc, c.toolReqs)
}

// onDone finalises a stream: on success the buffered text is committed to
// history and display; either way the stream channels are cleared.
func (c Chat) onDone(msg ChatDoneMsg) (tea.Model, tea.Cmd) {
	if msg.SessionID != c.sessionID {
		return c, nil
	}
	c.waiting = false
	if msg.Err != nil {
		c.status = "[warn] " + msg.Err.Error()
	} else {
		c.status = ""
		c.messages = append(c.messages, provider.Message{Role: "assistant", Content: c.streamBuf})
		c.display = append(c.display, assistantStyle.Render(c.sessionName+": ")+c.streamBuf)
		c.ctxChars += len(c.streamBuf)
	}
	c.streamBuf = ""
	c.streamTokens = nil
	c.streamStatus = nil
	c.streamErrc = nil
	c.toolReqs = nil
	c.toolApprovals = nil
	c.runningTool = ""
	c.loadingModel = ""
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
	return c, nil
}

// onTick advances the thinking spinner while a response is awaited.
func (c Chat) onTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if !c.waiting {
		return c, nil
	}
	var cmd tea.Cmd
	c.spinner, cmd = c.spinner.Update(msg)
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
	return c, cmd
}

// onCompact replaces the conversation with a single summary message produced by
// the /compact command.
func (c Chat) onCompact(msg compactHistoryResultMsg) (tea.Model, tea.Cmd) {
	c.waiting = false
	if msg.err != nil {
		c.status = "[warn] compact failed: " + msg.err.Error()
		return c, nil
	}
	c.messages = []provider.Message{{Role: "assistant", Content: msg.summary}}
	c.ctxChars = len(msg.summary)
	c.historyLoaded = true
	c.rebuildDisplay()
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
	c.status = ""
	return c, nil
}
