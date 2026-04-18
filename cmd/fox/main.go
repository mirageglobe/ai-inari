// fox is an interactive CLI companion to kitsune (TUI) and inarid (daemon).
// it runs inline in the terminal (no alt-screen) — like opencode or claude code —
// letting you pick a session and chat without leaving your shell.
//
// usage:
//
//	fox               start interactive session
//	fox sessions      list all sessions (one-shot)
//	fox ping          check if inarid is running (one-shot)
//	fox help          show usage
package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

const socketPath = "/tmp/inari.sock"

// ── styles ────────────────────────────────────────────────────────────────────

var (
	dimStyle  = lipgloss.NewStyle().Faint(true)
	boldStyle = lipgloss.NewStyle().Bold(true)
	foxHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	userLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	foxLabel  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	pickCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
)

// ── state machine ─────────────────────────────────────────────────────────────

type uiState int

const (
	stateLoading uiState = iota // fetching sessions from inarid
	statePicking                // user selects a session
	stateReady                  // input focused, waiting for user
	stateWaiting                // message sent, waiting for reply
	stateDone                   // clean exit
	stateFailed                 // unrecoverable error
)

// ── model ─────────────────────────────────────────────────────────────────────

type chatEntry struct {
	role    string // "you" or "fox"
	content string
}

type model struct {
	client    *ipc.Client
	state     uiState
	sessions  []ipc.SessionInfo
	cursor    int
	sessionID string
	sessName  string
	sessModel string
	history   []chatEntry
	input     textinput.Model
	sp        spinner.Model
	err       string
}

// ── tea messages ──────────────────────────────────────────────────────────────

type sessionsMsg struct {
	sessions []ipc.SessionInfo
	err      error
}

type replyMsg struct {
	content string
	err     error
}

// ── tea commands ──────────────────────────────────────────────────────────────

func fetchSessions(c *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		sessions, err := c.ListSessions()
		return sessionsMsg{sessions: sessions, err: err}
	}
}

func sendMessage(c *ipc.Client, sessionID, text string) tea.Cmd {
	return func() tea.Msg {
		reply, err := c.Chat(sessionID, text)
		return replyMsg{content: reply, err: err}
	}
}

// ── init ──────────────────────────────────────────────────────────────────────

func newModel(c *ipc.Client) model {
	ti := textinput.New()
	ti.Placeholder = "type a message…"
	ti.CharLimit = 4096

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	return model{client: c, state: stateLoading, input: ti, sp: sp}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchSessions(m.client), m.sp.Tick)
}

// ── update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd

	case sessionsMsg:
		if msg.err != nil {
			m.state = stateFailed
			m.err = msg.err.Error()
			return m, nil
		}
		if len(msg.sessions) == 0 {
			m.state = stateFailed
			m.err = "no sessions found — open kitsune and press [s] to create one"
			return m, nil
		}
		m.sessions = msg.sessions
		m.state = statePicking
		return m, nil

	case replyMsg:
		if msg.err != nil {
			m.state = stateFailed
			m.err = msg.err.Error()
			return m, nil
		}
		m.history = append(m.history, chatEntry{role: "fox", content: msg.content})
		m.state = stateReady
		m.input.Focus()
		return m, nil

	case tea.KeyMsg:
		switch m.state {

		case statePicking:
			switch msg.String() {
			case "ctrl+c", "q":
				m.state = stateDone
				return m, tea.Quit
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.sessions)-1 {
					m.cursor++
				}
			case "enter":
				sess := m.sessions[m.cursor]
				if sess.Model == "" {
					m.err = fmt.Sprintf("session %q has no model — open kitsune and press [m] to assign one", sess.Name)
					m.state = stateFailed
					return m, nil
				}
				m.sessionID = sess.ID
				m.sessName = sess.Name
				m.sessModel = sess.Model
				m.state = stateReady
				m.input.Focus()
			}

		case stateReady:
			switch msg.String() {
			case "ctrl+c":
				m.state = stateDone
				return m, tea.Quit
			case "enter":
				text := strings.TrimSpace(m.input.Value())
				if text == "" {
					return m, nil
				}
				m.history = append(m.history, chatEntry{role: "you", content: text})
				m.input.SetValue("")
				m.state = stateWaiting
				return m, tea.Batch(sendMessage(m.client, m.sessionID, text), m.sp.Tick)
			}

		case stateFailed:
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				return m, tea.Quit
			}
		}
	}

	if m.state == stateReady {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ── view ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	switch m.state {
	case stateLoading:
		return "\n  " + m.sp.View() + "  connecting to inarid…\n\n"

	case stateFailed:
		return "\n  " + errStyle.Render("error: "+m.err) + "\n\n"

	case stateDone:
		return ""

	case statePicking:
		return viewPicker(m)

	default:
		return viewChat(m)
	}
}

func viewPicker(m model) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + foxHeader.Render("fox") + "  " + dimStyle.Render("select a session") + "\n\n")
	for i, s := range m.sessions {
		cur := "   "
		name := s.Name
		mdl := s.Model
		if mdl == "" {
			mdl = "—"
		}
		if i == m.cursor {
			cur = pickCursor.Render("▸  ")
			name = boldStyle.Render(name)
		}
		b.WriteString(fmt.Sprintf("  %s%-22s  %s\n", cur, name, dimStyle.Render(mdl)))
	}
	b.WriteString("\n  " + dimStyle.Render("↑↓ / jk  navigate    enter  select    q  quit") + "\n\n")
	return b.String()
}

func viewChat(m model) string {
	var b strings.Builder
	b.WriteString("\n")

	// header
	b.WriteString("  " + foxHeader.Render("fox") +
		"  " + boldStyle.Render(m.sessName) +
		"  " + dimStyle.Render(m.sessModel) + "\n")
	b.WriteString("  " + dimStyle.Render(strings.Repeat("─", 60)) + "\n\n")

	// conversation history
	for _, entry := range m.history {
		if entry.role == "you" {
			b.WriteString("  " + userLabel.Render("you") + "  " + entry.content + "\n\n")
		} else {
			b.WriteString("  " + foxLabel.Render("fox") + "  " + entry.content + "\n\n")
		}
	}

	// waiting spinner
	if m.state == stateWaiting {
		b.WriteString("  " + m.sp.View() + "  thinking…\n\n")
	}

	// input row
	b.WriteString("  " + dimStyle.Render(strings.Repeat("─", 60)) + "\n")
	b.WriteString("  " + m.input.View() + "\n")
	b.WriteString("  " + dimStyle.Render("enter  send    ctrl+c  quit") + "\n\n")
	return b.String()
}

// ── one-shot commands ─────────────────────────────────────────────────────────

func dial() *ipc.Client {
	c := ipc.NewClient(socketPath)
	if err := c.TryReconnect(); err != nil {
		fatalf("inarid is not running — start it with `make start`")
	}
	return c
}

func cmdPing() {
	c := dial()
	defer c.Close()
	if err := c.Ping(); err != nil {
		fatalf("ping: %v", err)
	}
	fmt.Println("pong")
}

func cmdSessions() {
	c := dial()
	defer c.Close()
	sessions, err := c.ListSessions()
	if err != nil {
		fatalf("sessions: %v", err)
	}
	if len(sessions) == 0 {
		fmt.Println("no sessions")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "name\tid\tmodel")
	for _, s := range sessions {
		mdl := s.Model
		if mdl == "" {
			mdl = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.ID, mdl)
	}
	w.Flush()
}

func printUsage() {
	fmt.Print(`fox — interactive CLI for inari

usage:
  fox               start interactive session
  fox sessions      list all sessions
  fox ping          check if inarid is running
  fox help          show this help
`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "ping":
			cmdPing()
			return
		case "sessions":
			cmdSessions()
			return
		case "help", "--help", "-h":
			printUsage()
			return
		default:
			fatalf("unknown command %q — run `fox help` for usage", args[0])
		}
	}

	// no args → interactive mode (inline, no alt-screen)
	c := dial()
	defer c.Close()
	p := tea.NewProgram(newModel(c))
	if _, err := p.Run(); err != nil {
		fatalf("%v", err)
	}
}
