package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
// display holds the rendered lines shown in the viewport — local to this kitsune instance.
// all message history lives in inarid; kitsune sends only the new user text each turn.
// the waiting spinner is rendered separately and is never written into display.
// historyLoaded prevents duplicate appends when Init() is called more than once
// (e.g. returning to this chat after a model-selector round-trip).
// ctxChars tracks the raw character total of all user+assistant message content,
// used to estimate token usage (~4 chars per token) shown in the header.
// streamBuf accumulates in-progress tokens during an active stream; it is rendered
// live in the viewport and moved into display on ChatDoneMsg.
// streamTokens / streamErrc are the channels for the active stream goroutine;
// nil when no stream is in flight.
// offline mirrors the root model's connectivity state; when true, sends are blocked
// and the send command is visually disabled in the hint bar.
// cwd is non-empty when builtin tools (read_file, list_dir) are active for this session.
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
	status        string // transient status/warn message shown in the status line
	inputHistory  []string // sent user messages, oldest first
	historyIdx    int      // index into inputHistory during navigation; -1 = not navigating
	historyDraft  string   // saves the in-progress input when history navigation starts
	streamBuf     string
	streamTokens  <-chan string
	streamErrc    <-chan error
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
// so prior conversations are restored when kitsune reconnects to an existing session.
func (c Chat) Init() tea.Cmd {
	return tea.Batch(c.input.Focus(), fetchChatHistory(c.client, c.sessionID))
}

func (c Chat) SessionID() string   { return c.sessionID }
func (c Chat) SessionName() string { return c.sessionName }

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
		client:      client,
		sessionID:   sessionID,
		sessionName: sessionName,
		model:       model,
		cwd:         cwd,
		input:       ta,
		spinner:     sp,
		ctxChars:    ctxChars,
		historyIdx:  -1,
	}
}

// viewportContent returns the string to show in the viewport.
// during streaming, streamBuf is rendered as a live in-progress assistant message.
// before the first token arrives, the spinner is shown instead.
// neither is ever written into display so finalisation is a simple append.
func (c Chat) viewportContent() string {
	base := strings.Join(c.display, "\n")
	if c.streamBuf != "" {
		partial := assistantStyle.Render(c.sessionName+": ") + c.streamBuf
		if base == "" {
			return partial
		}
		return base + "\n" + partial
	}
	if c.waiting {
		thinking := thinkingStyle.Render(c.spinner.View() + " thinking…")
		if base == "" {
			return thinking
		}
		return base + "\n" + thinking
	}
	return base
}

// setViewportContent pre-wraps content to the viewport width before calling
// SetContent. bubbles v0.18.0 viewport splits content only on \n — it does not
// perform terminal line-wrapping itself — so GotoBottom undershoots when long
// styled lines wrap visually in the terminal. hardwrapping beforehand makes the
// \n count match the visual row count, fixing the scroll position.
func setViewportContent(vp *viewport.Model, content string) {
	if vp.Width > 0 {
		content = ansi.Hardwrap(content, vp.Width, true)
	}
	vp.SetContent(content)
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

// readNextToken returns a cmd that blocks until the next token arrives on the channel,
// then emits ChatTokenMsg or ChatDoneMsg (when the channel is closed).
func readNextToken(sessionID string, tokens <-chan string, errc <-chan error) tea.Cmd {
	return func() tea.Msg {
		token, ok := <-tokens
		if !ok {
			return ChatDoneMsg{SessionID: sessionID, Err: <-errc}
		}
		return ChatTokenMsg{SessionID: sessionID, Token: token}
	}
}

func (c *Chat) rebuildDisplay() {
	c.display = nil
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

func (c Chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ThemeChangedMsg:
		c.spinner.Style = spinnerStyle
		c.rebuildDisplay()
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

	case ChatTokenMsg:
		if msg.SessionID != c.sessionID {
			return c, nil
		}
		c.streamBuf += msg.Token
		c.waiting = false // hide spinner once first token arrives
		setViewportContent(&c.viewport, c.viewportContent())
		c.viewport.GotoBottom()
		return c, readNextToken(c.sessionID, c.streamTokens, c.streamErrc)

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
		// textarea(1) + statusLine(1) + hint(1) = h+6 total.
		height := msg.Height - 6
		if height < 1 {
			height = 1
		}
		// textarea and viewport expand to the full terminal width.
		// subtract 2 for the left+right border columns that each component adds.
		contentW := msg.Width
		c.input.SetWidth(contentW - 2)
		// viewport is 1 char narrower than the border interior to leave room for the scrollbar.
		if !c.ready {
			c.viewport = viewport.New(contentW-3, height)
			c.viewport.KeyMap = arrowOnlyKeyMap()
			c.ready = true
		} else {
			c.viewport.Width = contentW - 3
			c.viewport.Height = height
		}
		setViewportContent(&c.viewport, c.viewportContent())
		c.viewport.GotoBottom()
		return c, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+f" && c.cwd != "" {
			c.showBuiltin = !c.showBuiltin
			return c, nil
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
			go func() {
				err := c.client.ChatStream(c.sessionID, text, tokens)
				errc <- err
				close(tokens)
			}()
			c.streamTokens = tokens
			c.streamErrc = errc

			setViewportContent(&c.viewport, c.viewportContent())
			c.viewport.GotoBottom()
			return c, tea.Batch(readNextToken(c.sessionID, tokens, errc), c.spinner.Tick)
		}
	}

	var (
		vpCmd tea.Cmd
		taCmd tea.Cmd
	)
	c.viewport, vpCmd = c.viewport.Update(msg)
	c.input, taCmd = c.input.Update(msg)
	return c, tea.Batch(vpCmd, taCmd)
}

func (c Chat) handleSlashCommand(cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/model change":
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
	default:
		c.status = "[warn] unknown command: " + cmd
		return c, nil
	}
}

func (c Chat) View() string {
	// +2 accounts for the left+right border columns so the hint aligns with the body border.
	var hint string
	if c.showBuiltin {
		builtinStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary)
		dimStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
		hint = builtinStyle.Render("tools") + "  " +
			dimStyle.Render("read_file") + "  " +
			dimStyle.Render("list_dir") + "  " +
			dimStyle.Render("(sandboxed to cwd)")
	} else {
		var builtinHint HintCmd
		if c.cwd != "" {
			builtinHint = H("/tools")
		} else {
			builtinHint = HD("/tools")
		}

		sendHint := H("[enter] send")
		if c.offline {
			sendHint = HD("[enter] send")
		}

		hint = RenderHint([]HintCmd{
			sendHint,
			H("[↑↓] history"),
			HS(),
			H("/model change"),
			builtinHint,
			H("[esc] back"),
		}, c.viewport.Width+2)
	}
	scrollbar := RenderScrollbar(c.viewport)
	var viewContent string
	if scrollbar != "" {
		viewContent = lipgloss.JoinHorizontal(lipgloss.Top, c.viewport.View(), scrollbar)
	} else {
		viewContent = c.viewport.View()
	}
	body := herdStyle.Render(viewContent)
	sepStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
	metaStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary)
	sep := sepStyle.Render(" | ")
	model := c.model
	if model == "" {
		model = "—"
	}
	tokens := fmtTokens(c.ctxChars)
	if c.ctxChars == 0 {
		tokens = "—"
	}
	cwd := c.cwd
	if cwd == "" {
		cwd = "—"
	}
	statusLine := labelStyle.Render("chat") + sep +
		labelStyle.Render(c.sessionName) + sep +
		metaStyle.Render(model) + sep +
		metaStyle.Render(tokens) + sep +
		metaStyle.Render(cwd)
	if c.status != "" {
		statusLine += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(c.status)
	}
	return body + "\n" + statusLine + "\n" + c.input.View() + "\n" + hint
}
