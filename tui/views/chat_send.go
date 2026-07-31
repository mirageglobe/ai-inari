// chat_send.go owns starting a chat turn: recording the user message and
// launching the stream goroutine whose channels feed the token handlers. it does
// NOT own key handling (chat_keys.go) or the token handlers (chat_stream.go).

package views

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/inari/internal/ipc"
	"github.com/mirageglobe/inari/internal/provider"
)

// sendChat records a user message, starts the stream goroutine, and returns the
// cmd that begins draining tokens. the stream channels are stored on the struct so
// the ChatTokenMsg handlers can schedule the next read without threading them
// through each message. text is assumed non-empty, non-slash, and online-checked.
func (c Chat) sendChat(text string) (Chat, tea.Cmd) {
	c.inputHistory = append(c.inputHistory, text)
	c.historyIdx = -1
	c.historyDraft = ""
	c.messages = append(c.messages, provider.Message{Role: "user", Content: text})
	c.display = append(c.display, userStyle.Render("you: ")+text)
	c.ctxChars += len(text)
	c.input.Reset()
	c.status = ""
	// soft, non-blocking heads-up if the outgoing message looks like it carries a
	// secret; the message is still sent (the user's call). shown through streaming
	// until the reply completes.
	if looksLikeSecret(text) {
		c.status = "[warn] this message may contain a secret; sending anyway"
	}
	c.waiting = true
	c.loadingModel = ""
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
	return c, tea.Batch(readNextToken(c.sessionID, tokens, statuses, errc, toolReqs), c.spinner.Tick)
}

// runShell handles a `!`-prefixed line: it echoes the command to the transcript and
// dispatches session.shell to the daemon, which runs it via a real shell (sh -c) in
// the session cwd and records the output in history. the result returns as a
// shellResultMsg (onShell appends the output). the command echo mirrors the daemon's
// framing so a re-attach renders consistently.
func (c Chat) runShell(line string) (Chat, tea.Cmd) {
	c.display = append(c.display, userStyle.Render("$ ")+line)
	c.status = "running: " + line
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
	id := c.sessionID
	client := c.client
	return c, func() tea.Msg {
		out, err := client.Shell(id, line)
		return shellResultMsg{command: line, output: out, err: err}
	}
}
