package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// viewportContent returns the fully hardwrapped string to show in the viewport.
// during streaming, streamBuf is rendered as a live in-progress assistant message;
// the wrapped scrollback base is cached per stream (streamBaseWrapped) so each token
// re-wraps only the in-progress line, not all of history (P2). before the first token
// the spinner is shown instead. when a tool approval is pending, the pending tool
// call is shown in place of the spinner. neither is ever written into display so
// finalisation is a simple append. content is pre-wrapped here (setViewportContent no
// longer wraps) so the viewport's \n count matches the visual row count.
func (c *Chat) viewportContent() string {
	w := c.viewport.Width
	wrap := func(s string) string {
		if w > 0 {
			return ansi.Hardwrap(s, w, true)
		}
		return s
	}
	if c.streamBuf != "" {
		base := c.streamBaseWrapped(w)
		partial := wrap(assistantStyle.Render(c.sessionName+": ") + c.streamBuf)
		if base == "" {
			return partial
		}
		return base + "\n" + partial
	}
	base := wrap(strings.Join(c.display, "\n"))
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
		waitLine = wrap(waitLine)
		if base == "" {
			return waitLine
		}
		return base + "\n" + waitLine
	}
	return base
}

// streamBaseWrapped returns the display scrollback joined and hardwrapped, cached for
// the current stream so each token re-wraps only the in-progress line, not all of
// history (P2). display is immutable between a stream's tokens; the cache is keyed on
// viewport width and display length, so a mid-stream resize or a finalised message
// (which changes len(display)) refreshes it. onDone drops the cache at stream end.
func (c *Chat) streamBaseWrapped(width int) string {
	if width == c.streamBaseW && len(c.display) == c.streamBaseN {
		return c.streamBase
	}
	base := strings.Join(c.display, "\n")
	if width > 0 {
		base = ansi.Hardwrap(base, width, true)
	}
	c.streamBase = base
	c.streamBaseW = width
	c.streamBaseN = len(c.display)
	return base
}

// setViewportContent sets the viewport content, which viewportContent has already
// hardwrapped to the viewport width. bubbles v0.18.0 viewport splits content only on
// \n and does no line-wrapping itself, so GotoBottom would undershoot on visually
// wrapped lines; pre-wrapping (in viewportContent) keeps the \n count equal to the
// visual row count. the wrap lives in viewportContent, not here, so the streaming
// path can cache the wrapped scrollback base and re-wrap only the in-progress line.
func setViewportContent(vp *viewport.Model, content string) {
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
