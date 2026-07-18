// chat_stream.go owns the chat view's streaming and tool-call message handlers:
// token arrival, stream completion, tool-approval requests, spinner ticks, and
// compaction results. it does NOT own the top-level Update dispatch (chat.go).

package views

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/ipc"
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
// history and display; either way the stream channels are cleared. when the turn
// pushes context past the auto-compact threshold it fires the same summarisation
// pipeline /compact uses, without the user asking.
func (c Chat) onDone(msg ChatDoneMsg) (tea.Model, tea.Cmd) {
	if msg.SessionID != c.sessionID {
		return c, nil
	}
	c.waiting = false
	autoCompact := false
	if msg.Err != nil {
		c.status = "[warn] " + msg.Err.Error()
	} else {
		c.status = ""
		c.messages = append(c.messages, provider.Message{Role: "assistant", Content: c.streamBuf})
		c.display = append(c.display, assistantStyle.Render(c.sessionName+": ")+c.streamBuf)
		c.ctxChars += len(c.streamBuf)
		autoCompact = shouldAutoCompact(c.ctxChars, effectiveNumCtx(c.numCtxOverride, c.maxCtx))
	}
	c.streamBuf = ""
	c.streamBase = ""  // drop the per-stream wrapped-base cache (P2)
	c.streamBaseN = -1 // force a recompute on the next stream's first token
	c.streamTokens = nil
	c.streamStatus = nil
	c.streamErrc = nil
	c.toolReqs = nil
	c.toolApprovals = nil
	c.runningTool = ""
	c.loadingModel = ""
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
	if autoCompact {
		id := c.sessionID
		c.waiting = true
		c.status = "auto-compacting…"
		return c, tea.Batch(c.spinner.Tick, func() tea.Msg {
			summary, err := c.client.CompactHistory(id)
			return compactHistoryResultMsg{summary: summary, err: err}
		})
	}
	return c, nil
}

// autoCompactFraction is the share of the effective context window at which a
// session auto-compacts after a turn, keeping usage clear of the window without
// the user issuing /compact. a package const for now; daemon-configurable wiring
// is a follow-up (see SPEC roadmap).
const autoCompactFraction = 0.8

// shouldAutoCompact reports whether the running token estimate (ctxChars/4, the
// same estimate the footer shows) has reached autoCompactFraction of the effective
// context window. false when the window is unknown (<= 0), so a session with no
// detected window and no override never auto-compacts.
func shouldAutoCompact(ctxChars, window int) bool {
	if window <= 0 {
		return false
	}
	estTokens := ctxChars / 4
	return float64(estTokens) >= autoCompactFraction*float64(window)
}

// effectiveNumCtx is the context window inarid will actually request for a
// session: the per-session override when set (> 0), otherwise the model-derived
// capped default. mirrors the daemon's own precedence in handleStream.
func effectiveNumCtx(numCtxOverride, maxCtx int) int {
	if numCtxOverride > 0 {
		return numCtxOverride
	}
	return ipc.DefaultNumCtx(maxCtx)
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
