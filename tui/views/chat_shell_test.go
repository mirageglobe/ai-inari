package views

import (
	"errors"
	"strings"
	"testing"
)

// TestOnShellRecordsOutputAndHistory asserts a successful `!` result appends the
// output to the transcript and mirrors the command+output into local history as a
// single user message, so the model-visible context matches what the daemon recorded.
func TestOnShellRecordsOutputAndHistory(t *testing.T) {
	c := Chat{}
	m, _ := c.onShell(shellResultMsg{command: "echo hi", output: "hi\n"})
	got := m.(Chat)

	if joined := strings.Join(got.display, "\n"); !strings.Contains(joined, "hi") {
		t.Fatalf("output should appear in the transcript, got %q", joined)
	}
	if len(got.messages) != 1 {
		t.Fatalf("expected one mirrored history message, got %d", len(got.messages))
	}
	msg := got.messages[0]
	if msg.Role != "user" || !strings.Contains(msg.Content, "$ echo hi") || !strings.Contains(msg.Content, "hi") {
		t.Fatalf("history mirror should hold combined command+output as a user message, got %+v", msg)
	}
}

// TestOnShellErrorSurfacesStatus asserts a failed `!` RPC shows the error in the
// status line and records nothing in history.
func TestOnShellErrorSurfacesStatus(t *testing.T) {
	c := Chat{}
	m, _ := c.onShell(shellResultMsg{command: "false", err: errors.New("boom")})
	got := m.(Chat)

	if !strings.Contains(got.status, "boom") {
		t.Fatalf("error should surface in the status line, got %q", got.status)
	}
	if len(got.messages) != 0 {
		t.Fatal("a failed shell command must not record history")
	}
}
