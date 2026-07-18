package views

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/provider"
)

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
// contextLine is a one-line summary of the injected file-tree/project-context system
// prompt, rendered as the first line of display so the user can see what was loaded.
type Chat struct {
	client         *ipc.Client
	sessionID      string
	sessionName    string
	model          string // display only
	cwd            string
	contextLine    string
	messages       []provider.Message
	display        []string
	viewport       viewport.Model
	input          textarea.Model
	spinner        spinner.Model
	waiting        bool
	ready          bool
	historyLoaded  bool
	offline        bool
	showBuiltin    bool
	ctxChars       int
	maxCtx         int      // model's max context window (tokens); 0 until fetched / unknown
	numCtxOverride int      // per-session num_ctx override (tokens); 0 = use computed default
	status         string   // transient status/warn message shown in the status line
	inputHistory   []string // sent user messages, oldest first
	historyIdx     int      // index into inputHistory during navigation; -1 = not navigating
	historyDraft   string   // saves the in-progress input when history navigation starts
	inputFocused   bool
	streamBuf      string
	streamBase     string // wrapped display scrollback cached for the current stream (P2)
	streamBaseW    int    // viewport width streamBase was wrapped at
	streamBaseN    int    // len(display) streamBase was computed for
	streamTokens   <-chan string
	streamStatus   <-chan string
	streamErrc     <-chan error
	toolReqs       <-chan ipc.ToolRequestMsg
	toolApprovals  chan<- bool
	pendingTool    *toolApprovalRequestMsg
	runningTool    string // name of the tool currently executing after approval
	loadingModel   string // non-empty while inarid reports the assigned model is cold-loading
	selActive      bool
	selStartLine   int       // absolute content line (post-hardwrap) where drag started
	selEndLine     int       // absolute content line where drag currently ends
	lastActivity   time.Time // last keypress or stream token; drives the idle-hint timer
	idleHint       string    // rotating usage hint shown after idleHintDelay of inactivity
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
	cmds := []tea.Cmd{c.input.Focus(), fetchChatHistory(c.client, c.sessionID), fetchRecap(c.client, c.sessionID)}
	if c.model != "" {
		cmds = append(cmds, fetchModelContext(c.client, c.model))
	}
	return tea.Batch(cmds...)
}

func (c Chat) SessionID() string   { return c.sessionID }
func (c Chat) SessionName() string { return c.sessionName }
func (c Chat) Model() string       { return c.model }
func (c Chat) InputFocused() bool  { return c.inputFocused }

func NewChat(client *ipc.Client, sessionID, sessionName, model, cwd string, ctxChars, numCtxOverride int, systemPrompt string) Chat {
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
		client:         client,
		sessionID:      sessionID,
		sessionName:    sessionName,
		model:          model,
		cwd:            cwd,
		contextLine:    buildContextLine(cwd, systemPrompt),
		input:          ta,
		spinner:        sp,
		ctxChars:       ctxChars,
		numCtxOverride: numCtxOverride,
		historyIdx:     -1,
		inputFocused:   true,
		lastActivity:   time.Now(), // opening the chat counts as activity
	}
}
