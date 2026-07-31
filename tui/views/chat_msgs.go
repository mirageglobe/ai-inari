// chat_msgs.go owns the chat view's non-streaming result handlers: theme
// changes, export/unassign/clear results, history load, and window resize.
// it does NOT own streaming handlers (chat_stream.go) or the top-level Update
// dispatch (chat.go).

package views

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/inari/internal/provider"
)

// onThemeChanged restyles the spinner and rebuilds the rendered display so the
// new theme colours take effect.
func (c Chat) onThemeChanged() (tea.Model, tea.Cmd) {
	c.spinner.Style = spinnerStyle
	c.rebuildDisplay()
	return c, nil
}

// onThemeSaveErr surfaces a failed theme write in the status line.
func (c Chat) onThemeSaveErr(msg ThemeSaveErrMsg) (tea.Model, tea.Cmd) {
	c.status = "[warn] theme save failed: " + msg.Err.Error()
	return c, nil
}

// onSetCwd applies a /cwd result: on success it adopts the new working directory,
// rebuilds the context line from the returned info, and re-renders; tools become
// available since c.cwd is now set. on failure it surfaces the error in the status.
func (c Chat) onSetCwd(msg setCwdResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] cwd: " + msg.err.Error()
		return c, nil
	}
	c.cwd = msg.info.CWD
	c.contextLine = buildContextLine(msg.info.CWD, msg.info.SystemPrompt)
	c.rebuildDisplay()
	c.status = "[info] cwd -> " + msg.info.CWD
	return c, nil
}

// onShell applies a `!` shell result: on success it appends the command output to the
// transcript and mirrors the command+output into the local history as a single message
// (matching what the daemon recorded, so the model-visible context and the client
// agree); on failure it surfaces the error in the status line.
func (c Chat) onShell(msg shellResultMsg) (tea.Model, tea.Cmd) {
	c.status = ""
	if msg.err != nil {
		c.status = "[warn] shell: " + msg.err.Error()
		return c, nil
	}
	if out := strings.TrimRight(msg.output, "\n"); out != "" {
		c.display = append(c.display, out)
	}
	combined := "$ " + msg.command + "\n" + msg.output
	c.messages = append(c.messages, provider.Message{Role: "user", Content: combined})
	c.ctxChars += len(combined)
	setViewportContent(&c.viewport, c.viewportContent())
	c.viewport.GotoBottom()
	return c, nil
}

// onRename applies a /rename result: on success it adopts the new session name,
// refreshes the input placeholder, and rebuilds the display (assistant lines are
// prefixed with the session name); on failure it surfaces the error in the status.
func (c Chat) onRename(msg renameResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] rename: " + msg.err.Error()
		return c, nil
	}
	c.sessionName = msg.info.Name
	c.input.Placeholder = "message " + c.sessionName + " (" + c.model + ")..."
	c.rebuildDisplay()
	c.status = "[info] renamed -> " + c.sessionName
	return c, nil
}

// onTag applies a /tag result: on success it reports the session's current tag
// set in the status line; on failure it surfaces the error. tags themselves are
// displayed in the sessions view, which refreshes on its next list.
func (c Chat) onTag(msg tagResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] tag: " + msg.err.Error()
		return c, nil
	}
	if len(msg.info.Tags) == 0 {
		c.status = "[info] tags cleared"
	} else {
		c.status = "[info] tags: " + strings.Join(msg.info.Tags, " ")
	}
	return c, nil
}

// onSetNumCtx applies a /numctx result: adopts the new override so the footer
// window updates immediately, and reports the effective window in the status.
func (c Chat) onSetNumCtx(msg setNumCtxResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] numctx: " + msg.err.Error()
		return c, nil
	}
	c.numCtxOverride = msg.info.NumCtxOverride
	setViewportContent(&c.viewport, c.viewportContent())
	if c.numCtxOverride > 0 {
		c.status = "[info] num_ctx override -> " + strconv.Itoa(c.numCtxOverride)
	} else {
		c.status = "[info] num_ctx override cleared (using default)"
	}
	return c, nil
}

// onRole applies a /role result: on a successful auto-assign it adopts the
// recommended model (updating the placeholder and refetching its context window);
// otherwise it reports the role was set and names the model to pull.
func (c Chat) onRole(msg roleResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] role: " + msg.err.Error()
		return c, nil
	}
	if msg.assigned {
		c = c.WithModel(msg.model)
		c.input.Placeholder = "message " + c.sessionName + " (" + c.model + ")..."
		c.status = "[info] role " + msg.role + " -> model " + msg.model
		return c, fetchModelContext(c.client, c.model)
	}
	if msg.model == "" {
		c.status = "[info] role " + msg.role + " set (no curated model for this hardware tier)"
	} else {
		c.status = "[info] role " + msg.role + " set; recommended " + msg.model + " not pulled (use /model)"
	}
	return c, nil
}

// onExportResult reports where the session context was written, or why it failed.
func (c Chat) onExportResult(msg exportChatResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] save failed: " + msg.err.Error()
	} else {
		c.status = "[saved] " + msg.path
	}
	return c, nil
}

// onUnassign clears the displayed model after a successful model unload.
func (c Chat) onUnassign(msg unassignModelResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] unassign failed: " + msg.err.Error()
		return c, nil
	}
	c = c.WithModel("")
	c.status = "[info] model unloaded"
	return c, nil
}

// onClear empties the conversation after a successful /clear.
func (c Chat) onClear(msg clearHistoryResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		c.status = "[warn] clear failed: " + msg.err.Error()
		return c, nil
	}
	c.messages = nil
	c.ctxChars = 0
	c.historyLoaded = true
	c.rebuildDisplay()
	c.status = ""
	return c, nil
}

// onHistory loads prior session messages once, restoring a conversation on
// reconnect.
func (c Chat) onHistory(msg chatHistoryMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || c.historyLoaded {
		return c, nil
	}
	// mark loaded even when messages is empty; a new session has no history yet,
	// but historyLoaded must be true so a later Init() (e.g. after a model change)
	// does not re-append the now-populated history on top of what is already shown.
	c.historyLoaded = true
	if len(msg.messages) > 0 {
		c.messages = append(c.messages, msg.messages...)
	}
	// rebuild unconditionally so the pre-context line still renders for a new session.
	c.rebuildDisplay()
	return c, nil
}

// onWindowSize recomputes the viewport and input dimensions on a terminal resize.
func (c Chat) onWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
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
	// viewport fills the border interior; the right border is rendered separately by RenderRightEdge.
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
}
