package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/provider"
)

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	thinkingStyle  = lipgloss.NewStyle().Faint(true)
)

// ChatTokenMsg is sent for each streamed token from inarid.
// SessionID routes it to the correct Chat view regardless of which view is active.
type ChatTokenMsg struct {
	SessionID string
	Token     string
}

// ChatDoneMsg is sent when the stream ends (success or error).
type ChatDoneMsg struct {
	SessionID string
	Err       error
}

type chatHistoryMsg struct {
	messages []provider.Message
	err      error
}

// Chat is the interactive conversation view for a session.
// display holds the rendered lines shown in the viewport — local to this inari instance.
// all message history lives in inarid; inari sends only the new user text each turn.
// the waiting spinner is rendered separately and is never written into display.
// historyLoaded prevents duplicate appends when Init() is called more than once
// (e.g. returning to this chat after a model-selector round-trip).
// ctxChars tracks the raw character total of all user+assistant message content,
// used to estimate token usage (~4 chars per token) shown in the header.
// streamBuf accumulates in-progress tokens during an active stream; it is rendered
// live in the viewport and moved into display on ChatDoneMsg.
// streamTokens / streamErrc are the channels for the active stream goroutine;
// nil when no stream is in flight.
// toolReqs / toolApprovals are the tool-approval channels used while a stream is active.
// pendingTool is non-nil when the server has requested approval to run a tool; the
// TUI shows an approval prompt and the stream is paused until the user responds.
// offline mirrors the root model's connectivity state; when true, sends are blocked
// and the send command is visually disabled in the hint bar.
// cwd is non-empty when builtin tools (read_file, list_dir, grep_file, stat_file, run) are active for this session.
// showBuiltin toggles a builtin panel in the hint area listing available builtin tools.
type Chat struct {
	client        *ipc.Client
	sessionID     string
	sessionName   string
	model         string // display only
	cwd           string
	messages      []provider.Message
	display       []string
	viewport      viewport.Model
	input         textarea.Model
	spinner       spinner.Model
	waiting       bool
	ready         bool
	historyLoaded bool
	offline       bool
	showBuiltin   bool
	ctxChars      int
	status        string   // transient status/warn message shown in the status line
	inputHistory  []string // sent user messages, oldest first
	historyIdx    int      // index into inputHistory during navigation; -1 = not navigating
	historyDraft  string   // saves the in-progress input when history navigation starts
	inputFocused  bool
	streamBuf     string
	streamTokens  <-chan string
	streamErrc    <-chan error
	toolReqs      <-chan ipc.ToolRequestMsg
	toolApprovals chan<- bool
	pendingTool   *toolApprovalRequestMsg
	runningTool   string // name of the tool currently executing after approval
	selActive     bool
	selStartLine  int // absolute content line (post-hardwrap) where drag started
	selEndLine    int // absolute content line where drag currently ends
}

// WithOffline returns a copy of the chat with the offline flag set.
// called by the root model whenever connectivity changes.
func (c Chat) WithOffline(offline bool) Chat {
	c.offline = offline
	return c
}

func (c Chat) WithModel(model string) Chat {
	c.model = model
	return c
}

// Init focuses the textarea and fetches the session's message history from inarid
// so prior conversations are restored when inari reconnects to an existing session.
func (c Chat) Init() tea.Cmd {
	return tea.Batch(c.input.Focus(), fetchChatHistory(c.client, c.sessionID))
}

func (c Chat) SessionID() string   { return c.sessionID }
func (c Chat) SessionName() string { return c.sessionName }
func (c Chat) InputFocused() bool  { return c.inputFocused }

func NewChat(client *ipc.Client, sessionID, sessionName, model, cwd string, ctxChars int) Chat {
	ta := textarea.New()
	ta.Placeholder = "message " + sessionName + " (" + model + ")..."
	ta.Focus()
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.Prompt = "❯ "
	ta.CharLimit = 2048

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = thinkingStyle

	return Chat{
		client:       client,
		sessionID:    sessionID,
		sessionName:  sessionName,
		model:        model,
		cwd:          cwd,
		input:        ta,
		spinner:      sp,
		ctxChars:     ctxChars,
		historyIdx:   -1,
		inputFocused: true,
	}
}

// arrowOnlyKeyMap restricts viewport scrolling to arrow keys only,
// preventing vim bindings (k/j/g/G) from consuming keystrokes meant for the textarea.
func (c Chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ThemeChangedMsg:
		c.spinner.Style = spinnerStyle
		c.rebuildDisplay()
		return c, nil

	case ThemeSaveErrMsg:
		c.status = "[warn] theme save failed: " + msg.Err.Error()
		return c, nil

	case exportChatResultMsg:
		if msg.err != nil {
			c.status = "[warn] save failed: " + msg.err.Error()
		} else {
			c.status = "[saved] " + msg.path
		}
		return c, nil

	case clearHistoryResultMsg:
		if msg.err != nil {
			c.status = "[warn] clear failed: " + msg.err.Error()
			return c, nil
		}
		c.messages = nil
		c.display = nil
		c.ctxChars = 0
		c.historyLoaded = true
		setViewportContent(&c.viewport, c.viewportContent())
		c.status = ""
		return c, nil

	case compactHistoryResultMsg:
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

	case chatHistoryMsg:
		if msg.err != nil || c.historyLoaded {
			return c, nil
		}
		// mark loaded even when messages is empty — a new session has no history yet, but
		// historyLoaded must be true so that a later Init() (e.g. after model change) does
		// not re-append the now-populated history on top of what's already displayed.
		c.historyLoaded = true
		if len(msg.messages) == 0 {
			return c, nil
		}
		c.messages = append(c.messages, msg.messages...)
		c.rebuildDisplay()
		return c, nil

	case toolApprovalRequestMsg:
		if msg.SessionID != c.sessionID {
			return c, nil
		}
		c.waiting = false
		c.runningTool = ""
		c.pendingTool = &msg
		setViewportContent(&c.viewport, c.viewportContent())
		c.viewport.GotoBottom()
		return c, nil

	case ChatTokenMsg:
		if msg.SessionID != c.sessionID {
			return c, nil
		}
		c.streamBuf += msg.Token
		c.waiting = false // hide spinner once first token arrives
		c.runningTool = ""
		setViewportContent(&c.viewport, c.viewportContent())
		c.viewport.GotoBottom()
		return c, readNextToken(c.sessionID, c.streamTokens, c.streamErrc, c.toolReqs)

	case ChatDoneMsg:
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
		c.streamErrc = nil
		c.toolReqs = nil
		c.toolApprovals = nil
		c.runningTool = ""
		setViewportContent(&c.viewport, c.viewportContent())
		c.viewport.GotoBottom()
		return c, nil

	case spinner.TickMsg:
		if !c.waiting {
			return c, nil
		}
		var cmd tea.Cmd
		c.spinner, cmd = c.spinner.Update(msg)
		setViewportContent(&c.viewport, c.viewportContent())
		c.viewport.GotoBottom()
		return c, cmd

	case tea.WindowSizeMsg:
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
		// viewport fills the border interior; right border is rendered separately by RenderRightEdge.
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

	case tea.KeyMsg:
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
				return c, tea.Batch(readNextToken(c.sessionID, c.streamTokens, c.streamErrc, c.toolReqs), c.spinner.Tick)
			case "n", "N", "esc":
				c.toolApprovals <- false
				c.pendingTool = nil
				c.waiting = true
				setViewportContent(&c.viewport, c.viewportContent())
				c.viewport.GotoBottom()
				return c, tea.Batch(readNextToken(c.sessionID, c.streamTokens, c.streamErrc, c.toolReqs), c.spinner.Tick)
			}
			return c, nil // absorb other keys while approval is pending
		}
		// mark input as focused on any keypress; actual Focus() cmd is issued below after the switch.
		if !c.inputFocused {
			c.inputFocused = true
		}

		// ctrl-prefixed shortcuts never collide with typed text, so they stay active
		// while the input is focused (unlike bare keys like `t`/`?`; see open issue).
		// ctrl+m is deliberately omitted: terminals deliver it as carriage-return, which
		// would shadow [enter] send.
		switch msg.String() {
		case "ctrl+f", "ctrl+t":
			// toggle the builtin tools panel; only meaningful when tools are active.
			if c.cwd != "" {
				c.showBuiltin = !c.showBuiltin
			} else {
				c.status = "[warn] tools not available (no cwd set)"
			}
			return c, nil
		case "ctrl+g":
			// open the help overlay; the root model owns overlay visibility.
			return c, func() tea.Msg { return ToggleHelpMsg{} }
		case "ctrl+p":
			// open the slash command palette by seeding the input with "/".
			c.input.SetValue("/")
			c.input.CursorEnd()
			return c, nil
		case "esc":
			// esc exits the active entry mode: dismiss the tools panel or clear an
			// in-progress slash command, returning to plain chat entry.
			if c.showBuiltin {
				c.showBuiltin = false
				return c, nil
			}
			if strings.HasPrefix(c.input.Value(), "/") {
				c.input.Reset()
				return c, nil
			}
		}
		// up/down navigate input history instead of scrolling the viewport.
		if msg.String() == "up" && !c.waiting {
			if len(c.inputHistory) == 0 {
				return c, nil
			}
			if c.historyIdx == -1 {
				c.historyDraft = c.input.Value()
				c.historyIdx = len(c.inputHistory) - 1
			} else if c.historyIdx > 0 {
				c.historyIdx--
			}
			c.input.SetValue(c.inputHistory[c.historyIdx])
			c.input.CursorEnd()
			return c, nil
		}
		if msg.String() == "down" && !c.waiting {
			if c.historyIdx == -1 {
				return c, nil
			}
			if c.historyIdx < len(c.inputHistory)-1 {
				c.historyIdx++
				c.input.SetValue(c.inputHistory[c.historyIdx])
			} else {
				c.historyIdx = -1
				c.input.SetValue(c.historyDraft)
			}
			c.input.CursorEnd()
			return c, nil
		}
		if msg.Type == tea.KeyTab {
			inputVal := c.input.Value()
			if strings.HasPrefix(inputVal, "/") {
				for _, cmd := range chatCommands {
					if strings.HasPrefix(cmd, inputVal) {
						c.input.SetValue(cmd)
						c.input.CursorEnd()
						return c, nil
					}
				}
			}
			return c, nil
		}
		if msg.Type == tea.KeyEnter && !c.waiting {
			text := strings.TrimSpace(c.input.Value())
			if text == "" {
				return c, nil
			}
			if strings.HasPrefix(text, "/") {
				c.input.Reset()
				c.status = ""
				return c.handleSlashCommand(text)
			}
			if c.offline {
				return c, nil
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
			// start stream goroutine; store channels on struct so ChatTokenMsg
			// handlers can schedule the next readNextToken without carrying them in the message.
			tokens := make(chan string, 64)
			errc := make(chan error, 1)
			toolReqs := make(chan ipc.ToolRequestMsg, 1)
			approvals := make(chan bool, 1)
			go func() {
				err := c.client.ChatStream(c.sessionID, text, tokens, toolReqs, approvals)
				errc <- err
				close(tokens)
				close(toolReqs)
			}()
			c.streamTokens = tokens
			c.streamErrc = errc
			c.toolReqs = toolReqs
			c.toolApprovals = approvals

			setViewportContent(&c.viewport, c.viewportContent())
			c.viewport.GotoBottom()
			return c, tea.Batch(readNextToken(c.sessionID, tokens, errc, toolReqs), c.spinner.Tick)
		}

	case tea.MouseMsg:
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
					return c, c.input.Focus()
				}
				return c, nil
			}
		case tea.MouseActionMotion:
			if c.selActive {
				cl := c.viewport.YOffset + (msg.Y - viewportTopY)
				c.selEndLine = cl
				setViewportContentWithSel(&c.viewport, c.viewportContent(), c.selStartLine, c.selEndLine)
				return c, nil
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
				return c, nil
			}
		}
		// unhandled mouse events (wheel scroll etc.) fall through to viewport.Update below.
	}

	var (
		vpCmd    tea.Cmd
		taCmd    tea.Cmd
		focusCmd tea.Cmd
	)
	// apply input focus here so it covers both the fall-through key path and any other path
	// that set inputFocused without being able to return a cmd (e.g. early-return key cases).
	if c.inputFocused && !c.input.Focused() {
		focusCmd = c.input.Focus()
	}
	c.viewport, vpCmd = c.viewport.Update(msg)
	c.input, taCmd = c.input.Update(msg)
	return c, tea.Batch(vpCmd, taCmd, focusCmd)
}

func (c Chat) handleSlashCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/clear":
		id := c.sessionID
		return c, func() tea.Msg {
			return clearHistoryResultMsg{err: c.client.ClearHistory(id)}
		}
	case "/compact":
		id := c.sessionID
		c.waiting = true
		c.status = "compacting…"
		return c, tea.Batch(c.spinner.Tick, func() tea.Msg {
			summary, err := c.client.CompactHistory(id)
			return compactHistoryResultMsg{summary: summary, err: err}
		})
	case "/copy":
		// copy the most recent assistant response to the system clipboard.
		text := c.lastAssistantText()
		if text == "" {
			c.status = "[warn] no response to copy"
			return c, nil
		}
		if err := copyToClipboard(text); err != nil {
			c.status = "[warn] copy failed: " + err.Error()
		} else {
			c.status = "[copied] response"
		}
		return c, nil
	case "/export":
		// download the full session context (history) to a text file; reuses the
		// agents export path so both entry points write to the same location.
		return c, exportChatCmd(c.client, c.sessionID, c.sessionName)
	case "/model select":
		return c, func() tea.Msg {
			return OpenModelSelectorMsg{SessionID: c.sessionID, SessionName: c.sessionName}
		}
	case "/tools":
		if c.cwd != "" {
			c.showBuiltin = !c.showBuiltin
		} else {
			c.status = "[warn] tools not available (no cwd set)"
		}
		return c, nil
	case "/describe":
		return c, func() tea.Msg { return OpenDescribeMsg{} }
	case "/agents":
		return c, func() tea.Msg { return BackToAgentsMsg{} }
	case "/quit":
		return c, tea.Quit
	default:
		c.status = "[warn] unknown command: " + cmd
		return c, nil
	}
}
