package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestChatViewRenders exercises the chat render seam: after a window-size, View
// must render without panicking, be non-empty, and include the session name.
func TestChatViewRenders(t *testing.T) {
	c := NewChat(nil, "id1", "myagent", "gemma4:e2b", "", 0, 0, "")
	m, _ := c.onWindowSize(tea.WindowSizeMsg{Width: 80, Height: 24})
	c = m.(Chat)
	out := c.View()
	if out == "" {
		t.Fatal("chat View rendered empty")
	}
	if !strings.Contains(out, "myagent") {
		t.Fatalf("chat View missing session name; got:\n%s", out)
	}
}

// TestAgentsViewRenders exercises the agents render seam: after a window-size,
// View must render without panicking and be non-empty even with no sessions.
func TestAgentsViewRenders(t *testing.T) {
	h := NewAgents(nil)
	m, _ := h.onWindowSize(tea.WindowSizeMsg{Width: 100, Height: 30})
	h = m.(Agents)
	if out := h.View(); out == "" {
		t.Fatal("agents View rendered empty")
	}
}
