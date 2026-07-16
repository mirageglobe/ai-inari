package views

import (
	"strings"
	"testing"
)

// TestModalRendersNonEmpty smoke-tests the new popup renderers: they must produce
// output without panicking at a normal terminal size. guards the render path the
// state-transition tests (in package tui) don't exercise.
func TestModalRendersNonEmpty(t *testing.T) {
	if got := NewLogs().RenderModal(80, 24); strings.TrimSpace(got) == "" {
		t.Error("logs RenderModal produced empty output")
	}
	if got := NewDescribe().RenderModal(80, 24); strings.TrimSpace(got) == "" {
		t.Error("describe RenderModal produced empty output")
	}
	if got := (Chat{}).toolsModal(60, 20); !strings.Contains(got, "builtin tools") {
		t.Errorf("tools modal should list builtin tools, got %q", got)
	}
}
