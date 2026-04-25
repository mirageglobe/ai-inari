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

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/ollama"
)

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	thinkingStyle  = lipgloss.NewStyle().Faint(true)
)

type chatReplyMsg struct {
	text string
	err  error
}

type chatHistoryMsg struct {
	messages []ollama.Message
	err      error
}

// Chat is the interactive conversation view for a session.
// display holds the rendered lines shown in the viewport — local to this fox instance.
// all message history lives in inarid; fox sends only the new user text each turn.
// the waiting spinner is rendered separately and is never written into display.
// historyLoaded prevents duplicate appends when Init() is called more than once
// (e.g. returning to this chat after a model-selector round-trip).
// ctxChars tracks the raw character total of all user+assistant message content,
// used to estimate token usage (~4 chars per token) shown in the header.
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
}

// Init focuses the textarea and fetches the session's message history from inarid
// so prior conversations are restored when fox reconnects to an existing session.
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
// when waiting, the animated spinner line is appended; it is never stored in display
// so that chatReplyMsg can simply append the real reply without any pop logic.
func (c Chat) viewportContent() string {
	base := strings.Join(c.display, "\n")
	if !c.waiting {
		return base
	}
	thinking := thinkingStyle.Render(c.spinner.View() + " thinking…")
	if base == "" {
		return thinking
	}
	return base + "\n" + thinking
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
		c.viewport.SetContent(c.viewportContent())
		c.viewport.GotoBottom()
		return c, nil

	case chatReplyMsg:
		c.waiting = false
		if msg.err != nil {
			c.display = append(c.display, errorStyle.Render("error: "+msg.err.Error()))
		} else {
			c.display = append(c.display, assistantStyle.Render(c.sessionName+": ")+msg.text)
			c.ctxChars += len(msg.text)
		}
		c.viewport.SetContent(c.viewportContent())
		c.viewport.GotoBottom()
		return c, nil

	case spinner.TickMsg:
		if !c.waiting {
			return c, nil
		}
		var cmd tea.Cmd
		c.spinner, cmd = c.spinner.Update(msg)
		c.viewport.SetContent(c.viewportContent())
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
		c.viewport.SetContent(c.viewportContent())
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
			c.viewport.SetContent(c.viewportContent())
			c.viewport.GotoBottom()
			return c, tea.Batch(sendMessage(c.client, c.sessionID, text), c.spinner.Tick)
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

func sendMessage(client *ipc.Client, sessionID, text string) tea.Cmd {
	return func() tea.Msg {
		reply, err := client.Chat(sessionID, text)
		if err != nil {
			return chatReplyMsg{err: err}
		}
		return chatReplyMsg{text: reply}
	}
}
