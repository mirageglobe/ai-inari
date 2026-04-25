package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/ollama"
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
	messages []ollama.Message
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
type Chat struct {
	client        *ipc.Client
	sessionID     string
	sessionName   string
	model         string // display only
	display       []string
	viewport      viewport.Model
	input         textarea.Model
	spinner       spinner.Model
	waiting       bool
	ready         bool
	historyLoaded bool
	ctxChars      int
	streamBuf     string
	streamTokens  <-chan string
	streamErrc    <-chan error
}

// Init focuses the textarea and fetches the session's message history from inarid
// so prior conversations are restored when kitsune reconnects to an existing session.
func (c Chat) Init() tea.Cmd {
	return tea.Batch(c.input.Focus(), fetchChatHistory(c.client, c.sessionID))
}

func (c Chat) SessionID() string   { return c.sessionID }
func (c Chat) SessionName() string { return c.sessionName }

func NewChat(client *ipc.Client, sessionID, sessionName, model string, ctxChars int) Chat {
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
		input:       ta,
		spinner:     sp,
		ctxChars:    ctxChars,
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

// fmtTokens formats a character count as a human-readable estimated token count.
// uses the ~4 chars-per-token rule of thumb common for English/code text.
func fmtTokens(chars int) string {
	t := chars / 4
	if t < 1000 {
		return fmt.Sprintf("~%d tokens", t)
	}
	return fmt.Sprintf("~%.1fk tokens", float64(t)/1000)
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
		Up:           key.NewBinding(key.WithKeys("up")),
		Down:         key.NewBinding(key.WithKeys("down")),
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

func (c Chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
		for _, m := range msg.messages {
			switch m.Role {
			case "user":
				c.display = append(c.display, userStyle.Render("you: ")+m.Content)
			case "assistant":
				c.display = append(c.display, assistantStyle.Render(c.sessionName+": ")+m.Content)
			}
		}
		setViewportContent(&c.viewport, c.viewportContent())
		c.viewport.GotoBottom()
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
			c.display = append(c.display, errorStyle.Render("error: "+msg.Err.Error()))
		} else {
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
		// topbar(1) + chat header(1) + border-top(1) + viewport(h) + border-bottom(1) +
		// textarea(1, no border — Base style is plain lipgloss.NewStyle()) + hint(1) = h+6 total.
		height := msg.Height - 6
		if height < 1 {
			height = 1
		}
		// textarea and viewport expand to the full terminal width.
		// subtract 2 for the left+right border columns that each component adds.
		contentW := msg.Width
		c.input.SetWidth(contentW - 2)
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
		if msg.Type == tea.KeyEnter && !c.waiting {
			text := strings.TrimSpace(c.input.Value())
			if text == "" {
				return c, nil
			}
			c.display = append(c.display, userStyle.Render("you: ")+text)
			c.ctxChars += len(text)
			c.input.Reset()
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

func (c Chat) View() string {
	title := c.model
	if c.sessionName != "" {
		title = c.sessionName + "  " + lipgloss.NewStyle().Faint(true).Render("("+c.model+")")
	}
	ctxStat := lipgloss.NewStyle().Faint(true).Render(fmtTokens(c.ctxChars))
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render("chat") +
		"  " + lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(title) +
		"  " + ctxStat
	// +2 accounts for the left+right border columns so the hint aligns with the body border.
	hint := RenderHint([]HintCmd{
		H("[enter] send"),
		H("[ctrl+o] model"),
		HS(),
		H("[↑↓] scroll"),
		H("[esc] back"),
	}, c.viewport.Width+2)
	body := herdStyle.Render(c.viewport.View())
	return header + "\n" + body + "\n" + c.input.View() + "\n" + hint
}
