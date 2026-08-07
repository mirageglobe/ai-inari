// chat_view.go owns the chat view's full-screen layout: the input prompt prefix
// and the View method that assembles the viewport, session/cwd/status footer, and
// hint bar. it does NOT own viewport content rendering or selection (chat_render.go).

package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// inputPrompt returns the entry prefix reflecting the active mode:
// [sh] while shell mode is active, [cmd] while composing a slash command, [tool]
// while the builtin panel is open, otherwise plain [chat]. gives the user visual
// feedback on what the input does.
func (c Chat) inputPrompt() string {
	switch {
	// shell mode is checked first, and before the slash case, because it is sticky
	// state rather than a property of the buffer: with the mode on, even a line
	// starting with `/` is sent to the shell, so the prompt must say so.
	case c.shellMode:
		return "[sh] ❯ "
	case strings.HasPrefix(c.input.Value(), "/"):
		return "[cmd] ❯ "
	case strings.HasPrefix(c.input.Value(), "!"):
		return "[sh] ❯ "
	case c.showBuiltin:
		return "[tool] ❯ "
	default:
		return "[chat] ❯ "
	}
}

// toolsModal renders the builtin-tools list as a centred popup over the chat body.
// it lists each tool with a one-line description; q/esc (chat_keys) close it.
func (c Chat) toolsModal(width, height int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary)
	nameStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary)
	descStyle := lipgloss.NewStyle().Faint(true)

	tools := []struct{ name, desc string }{
		{"read_file", "read a file's contents"},
		{"list_dir", "list a directory"},
		{"grep_file", "search files for a pattern"},
		{"stat_file", "file metadata (size, mtime)"},
		{"execute_shell_command", "run an allowlisted command"},
	}
	lines := []string{titleStyle.Render("builtin tools"), ""}
	for _, t := range tools {
		lines = append(lines, nameStyle.Render(fmt.Sprintf("%-22s", t.name))+descStyle.Render(t.desc))
	}
	lines = append(lines, "", descStyle.Render("sandboxed to the session cwd    [q/esc] close"))

	return renderModalBox(lines, width, height)
}

func (c Chat) View() string {
	// +2 accounts for the left+right border columns so the hint aligns with the body border.
	c.input.Prompt = c.inputPrompt()
	var hintLine string
	if inputVal := c.input.Value(); strings.HasPrefix(inputVal, "/") && !c.showBuiltin && c.pendingTool == nil {
		hintLine = c.renderChatSuggestions(inputVal, c.viewport.Width+2)
	}
	chatBoxStyle := sessionsStyle.BorderRight(false).BorderTop(true).BorderBottom(true).BorderLeft(true)
	rightEdge := RenderRightEdge(c.viewport)
	// while the tools modal is open it replaces the transcript body (centred popup);
	// q/esc close it (handled in chat_keys).
	viewportBody := c.viewport.View()
	if c.showBuiltin {
		viewportBody = c.toolsModal(c.viewport.Width, c.viewport.Height)
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, chatBoxStyle.Render(viewportBody), rightEdge)

	model := c.model
	if model == "" {
		model = "-"
	}
	tokens := fmtTokens(c.ctxChars)
	if c.ctxChars == 0 {
		tokens = "-"
	}
	// append the effective context window (num_ctx inarid requests) over the
	// model's max, once known: e.g. "~500 tokens  ctx 8192/40960". a per-session
	// override wins over the computed default and is shown even before the model's
	// max window is detected.
	if eff := effectiveNumCtx(c.numCtxOverride, c.maxCtx); eff > 0 {
		if c.maxCtx > 0 {
			tokens += fmt.Sprintf("  ctx %d/%d", eff, c.maxCtx)
		} else {
			tokens += fmt.Sprintf("  ctx %d", eff)
		}
	}
	sessionLine := RenderSessionLine("chat", c.sessionName, model, tokens)
	cwdLine := renderCWDLine(c.cwd)

	var statusContent string
	if c.pendingTool != nil {
		warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		statusContent = warnStyle.Render("tool: "+c.pendingTool.Name) + "  " +
			thinkingStyle.Render(formatToolArgs(c.pendingTool.Args)) + "  " +
			activeStyle.Render("[y]") + " approve  " +
			activeStyle.Render("[n]") + " deny"
	} else if c.runningTool != "" {
		toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		statusContent = toolStyle.Render("[ tool ] " + c.runningTool)
	} else if c.status != "" {
		statusContent = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(c.status)
	} else if c.idleHint != "" {
		// idle usage hint: dimmed, lowest priority so any real status wins.
		statusContent = lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true).Render("hint: " + c.idleHint)
	}
	return body + "\n" + renderFooter(sessionLine, cwdLine, renderStatusLine(statusContent), c.input.View(), hintLine)
}
