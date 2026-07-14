package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

type clearHistoryResultMsg struct{ err error }

// setCwdResultMsg carries the outcome of a /cwd command: the updated session info
// (new cwd + rebuilt system prompt) on success, or an error.
type setCwdResultMsg struct {
	info ipc.SessionInfo
	err  error
}
type compactHistoryResultMsg struct {
	summary string
	err     error
}

// renameResultMsg carries the outcome of a /rename command: the updated session
// info (new name) on success, or an error.
type renameResultMsg struct {
	info ipc.SessionInfo
	err  error
}

// tagResultMsg carries the outcome of a /tag command: the updated session info
// (new tag set) on success, or an error.
type tagResultMsg struct {
	info ipc.SessionInfo
	err  error
}

// setNumCtxResultMsg carries the outcome of a /numctx command: the updated
// session info (new num_ctx override) on success, or an error.
type setNumCtxResultMsg struct {
	info ipc.SessionInfo
	err  error
}

// roleResultMsg carries the outcome of a /role command: the role set, the model
// recommended for it, and whether that model was successfully assigned (false
// when it is not pulled yet). err is set only when setting the role itself failed.
type roleResultMsg struct {
	role     string
	model    string
	assigned bool
	err      error
}

// toolApprovalRequestMsg is emitted when the server wants to run a tool and needs user approval.
type toolApprovalRequestMsg struct {
	SessionID string
	Name      string
	Args      map[string]any
}

// arrowOnlyKeyMap restricts viewport scrolling to arrow keys only,
// preventing vim bindings (k/j/g/G) from consuming keystrokes meant for the textarea.
func arrowOnlyKeyMap() viewport.KeyMap {
	return viewport.KeyMap{
		PageDown:     key.NewBinding(key.WithKeys()),
		PageUp:       key.NewBinding(key.WithKeys()),
		HalfPageUp:   key.NewBinding(key.WithKeys()),
		HalfPageDown: key.NewBinding(key.WithKeys()),
		Up:           key.NewBinding(key.WithKeys()),
		Down:         key.NewBinding(key.WithKeys()),
	}
}

func fetchChatHistory(client *ipc.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		messages, err := client.History(sessionID)
		return chatHistoryMsg{messages: messages, err: err}
	}
}

// fetchModelContext looks up the model's max context window (tokens) via inarid,
// once, when a chat opens; a lookup failure yields 0 (window hidden in the footer).
func fetchModelContext(client *ipc.Client, model string) tea.Cmd {
	return func() tea.Msg {
		n, _ := client.ModelContextLength(model)
		return modelContextMsg{max: n}
	}
}

// fetchRecap asks inarid for a "where you left off" recap when a chat opens.
// inarid returns "" unless the session is idle 10+ min with a real conversation,
// so this is a no-op for fresh/active sessions. newlines are collapsed so the
// recap fits the single-line status slot.
func fetchRecap(client *ipc.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		text, _ := client.Recap(sessionID)
		text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
		return recapMsg{text: text}
	}
}

// formatToolArgs renders a tool argument map as a compact key=value string for display.
func formatToolArgs(args map[string]any) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, args[k]))
	}
	return strings.Join(parts, ", ")
}

// readNextToken returns a cmd that blocks until the next token, status update, or
// tool request arrives, then emits ChatTokenMsg, ChatStatusMsg, ChatDoneMsg (any
// channel closed), or toolApprovalRequestMsg. selecting on a nil toolReqs channel
// blocks indefinitely, effectively ignoring it.
func readNextToken(sessionID string, tokens <-chan string, statuses <-chan string, errc <-chan error, toolReqs <-chan ipc.ToolRequestMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case token, ok := <-tokens:
			if !ok {
				return ChatDoneMsg{SessionID: sessionID, Err: <-errc}
			}
			return ChatTokenMsg{SessionID: sessionID, Token: token}
		case status, ok := <-statuses:
			if !ok {
				return ChatDoneMsg{SessionID: sessionID, Err: <-errc}
			}
			return ChatStatusMsg{SessionID: sessionID, Status: status}
		case req, ok := <-toolReqs:
			if !ok {
				return ChatDoneMsg{SessionID: sessionID, Err: <-errc}
			}
			return toolApprovalRequestMsg{SessionID: sessionID, Name: req.Name, Args: req.Args}
		}
	}
}

// interruptStream fires the session.interrupt RPC over the shared client
// connection, aborting an in-flight response. it is fire-and-forget: the daemon
// cancels the generation and the stream ends via its normal done path, so no
// message is emitted here (nil).
func interruptStream(client *ipc.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		_ = client.Interrupt(sessionID)
		return nil
	}
}

// buildContextLine summarises the injected file-tree/project-context system prompt
// as a single line, so the user can see what context inarid loaded for this session.
// empty when cwd is unset, since no context is injected in that case.
func buildContextLine(cwd, systemPrompt string) string {
	if cwd == "" {
		return ""
	}
	line := "cwd: " + cwd
	if strings.Contains(systemPrompt, "\nproject context:\n") {
		line += " (+ project context)"
	}
	return thinkingStyle.Render("[context] " + line)
}

func (c *Chat) rebuildDisplay() {
	c.display = nil
	if c.contextLine != "" {
		c.display = append(c.display, c.contextLine)
	}
	for _, m := range c.messages {
		switch m.Role {
		case "user":
			c.display = append(c.display, userStyle.Render("you: ")+m.Content)
		case "assistant":
			c.display = append(c.display, assistantStyle.Render(c.sessionName+": ")+m.Content)
		case "error":
			c.display = append(c.display, errorStyle.Render("error: "+m.Content))
		}
	}
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
}
