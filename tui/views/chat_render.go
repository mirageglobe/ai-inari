package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

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
		switch {
		case c.runningTool != "":
			waitLine = thinkingStyle.Render(c.spinner.View() + " running: " + c.runningTool + "...")
		case c.loadingModel != "":
			waitLine = thinkingStyle.Render(c.spinner.View() + " loading " + c.loadingModel + "...")
		default:
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
