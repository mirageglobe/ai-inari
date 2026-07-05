package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// chatCommands is the ordered list of slash commands available in the chat view.
var chatCommands = []string{"/clear", "/compact", "/copy", "/export", "/model select", "/describe", "/tools", "/agents", "/quit"}

// viewportContent returns the string to show in the viewport.
// during streaming, streamBuf is rendered as a live in-progress assistant message.
// before the first token arrives, the spinner is shown instead.
// when a tool approval is pending, the pending tool call is shown in place of the spinner.
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
		var waitLine string
		if c.runningTool != "" {
			waitLine = thinkingStyle.Render(c.spinner.View() + " running: " + c.runningTool + "...")
		} else {
			waitLine = thinkingStyle.Render(c.spinner.View() + " thinking...")
		}
		if base == "" {
			return waitLine
		}
		return base + "\n" + waitLine
	}
	return base
}

// setViewportContent pre-wraps content to the viewport width before calling
// SetContent. bubbles v0.18.0 viewport splits content only on \n; it does not
// perform terminal line-wrapping itself, so GotoBottom undershoots when long
// styled lines wrap visually in the terminal. hardwrapping beforehand makes the
// \n count match the visual row count, fixing the scroll position.
func setViewportContent(vp *viewport.Model, content string) {
	if vp.Width > 0 {
		content = ansi.Hardwrap(content, vp.Width, true)
	}
	vp.SetContent(content)
}

// setViewportContentWithSel hardwraps content, applies a highlight background to
// lines [lo, hi] (inclusive, normalised), then sets the viewport content directly
// without a second hardwrap pass.
func setViewportContentWithSel(vp *viewport.Model, content string, lo, hi int) {
	if vp.Width > 0 {
		content = ansi.Hardwrap(content, vp.Width, true)
	}
	lines := strings.Split(content, "\n")
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("240"))
	for i := lo; i <= hi; i++ {
		lines[i] = selStyle.Render(lines[i])
	}
	vp.SetContent(strings.Join(lines, "\n"))
}

// selectedText extracts the plain text for the current selection range.
func (c Chat) selectedText() string {
	content := c.viewportContent()
	if c.viewport.Width > 0 {
		content = ansi.Hardwrap(content, c.viewport.Width, true)
	}
	lines := strings.Split(content, "\n")
	lo, hi := c.selStartLine, c.selEndLine
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	if lo > hi {
		return ""
	}
	return ansi.Strip(strings.Join(lines[lo:hi+1], "\n"))
}

// renderChatSuggestions replaces the hint bar when the user is typing a slash command.
// commands that match the current prefix are shown active; others are dimmed.
func renderChatSuggestions(prefix string, width int) string {
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	var cmds []HintCmd
	for _, cmd := range chatCommands {
		if strings.HasPrefix(cmd, prefix) {
			cmds = append(cmds, HintCmd{Label: cmd, Enabled: true})
		} else {
			cmds = append(cmds, HintCmd{Label: cmd, Enabled: false})
		}
	}

	const gap = "  "
	const prefixRaw = "cmd: "
	label := labelStyle.Render(prefixRaw)
	var parts []string
	for _, c := range cmds {
		style := activeStyle
		if !c.Enabled {
			style = dimStyle
		}
		parts = append(parts, style.Render(c.Label))
	}
	_ = width
	return label + strings.Join(parts, gap)
}

// lastAssistantText returns the content of the most recent assistant message,
// or "" if the conversation has no assistant reply yet.
func (c Chat) lastAssistantText() string {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == "assistant" {
			return c.messages[i].Content
		}
	}
	return ""
}

// inputPrompt returns the entry prefix reflecting the active mode:
// [/] while composing a slash command, [tool] while the builtin panel is open,
// otherwise plain [chat]. gives the user visual feedback on what the input does.
func (c Chat) inputPrompt() string {
	switch {
	case strings.HasPrefix(c.input.Value(), "/"):
		return "[/] ❯ "
	case c.showBuiltin:
		return "[tool] ❯ "
	default:
		return "[chat] ❯ "
	}
}

func (c Chat) View() string {
	// +2 accounts for the left+right border columns so the hint aligns with the body border.
	c.input.Prompt = c.inputPrompt()
	var hintLine string
	if inputVal := c.input.Value(); strings.HasPrefix(inputVal, "/") && !c.showBuiltin && c.pendingTool == nil {
		hintLine = renderChatSuggestions(inputVal, c.viewport.Width+2)
	} else if c.showBuiltin {
		builtinStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary)
		dimStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Faint(true)
		hintLine = builtinStyle.Render("tools") + "  " +
			dimStyle.Render("read_file") + "  " +
			dimStyle.Render("list_dir") + "  " +
			dimStyle.Render("grep_file") + "  " +
			dimStyle.Render("stat_file") + "  " +
			dimStyle.Render("run") + "  " +
			dimStyle.Render("(sandboxed to cwd)")
	} else {
		sendHint := H("[enter] send")
		if c.offline {
			sendHint = HD("[enter] send")
		}

		toolsHint := H("[ctrl+t] tools")
		if c.cwd == "" {
			toolsHint = HD("[ctrl+t] tools")
		}
		hintLine = RenderHint([]HintCmd{
			sendHint,
			H("[uparrow][downarrow] history"),
			HS(),
			toolsHint,
			H("[ctrl+g] help"),
			H("/agents"),
		}, c.viewport.Width+1)
	}
	chatBoxStyle := agentsStyle.BorderRight(false).BorderTop(true).BorderBottom(true).BorderLeft(true)
	rightEdge := RenderRightEdge(c.viewport)
	body := lipgloss.JoinHorizontal(lipgloss.Top, chatBoxStyle.Render(c.viewport.View()), rightEdge)

	model := c.model
	if model == "" {
		model = "-"
	}
	tokens := fmtTokens(c.ctxChars)
	if c.ctxChars == 0 {
		tokens = "-"
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
	}
	return body + "\n" + renderFooter(sessionLine, cwdLine, renderStatusLine(statusContent), c.input.View(), hintLine)
}
