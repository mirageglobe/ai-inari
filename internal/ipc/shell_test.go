package ipc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirageglobe/inari/internal/session"
)

// TestRunUserShellPipes asserts the `!` path runs through a real shell, so pipes
// work - this is the whole reason it uses sh -c instead of execTool's word-split path.
func TestRunUserShellPipes(t *testing.T) {
	got := strings.TrimSpace(runUserShell(t.TempDir(), "echo hello | tr a-z A-Z"))
	if got != "HELLO" {
		t.Fatalf("pipe not honored: want %q, got %q", "HELLO", got)
	}
}

// TestRunUserShellCwd asserts the command runs inside the given cwd.
func TestRunUserShellCwd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("here"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(runUserShell(dir, "cat marker.txt"))
	if got != "here" {
		t.Fatalf("command did not run in cwd: want %q, got %q", "here", got)
	}
}

// TestRunUserShellTruncate asserts output past the 64KB cap is truncated.
func TestRunUserShellTruncate(t *testing.T) {
	got := runUserShell(t.TempDir(), "head -c 70000 /dev/zero | tr '\\000' 'a'")
	if !strings.HasSuffix(got, "(truncated)") {
		t.Fatalf("oversized output should be truncated, got %d bytes ending %q", len(got), tail(got, 20))
	}
}

// TestSessionShellRecordsHistory drives session.shell end to end and asserts the
// command output returns to the client and is recorded in history for the model.
func TestSessionShellRecordsHistory(t *testing.T) {
	srv, client := newSetCwdServer(t, "/tmp/inari-test-shell-history.sock")
	defer client.Close()

	sess := session.New("test")
	sess.CWD = t.TempDir()
	srv.store.Add(sess)

	out, err := client.Shell(sess.ID, "echo hi")
	if err != nil {
		t.Fatalf("Shell: %v", err)
	}
	if strings.TrimSpace(out) != "hi" {
		t.Fatalf("output: want %q, got %q", "hi", out)
	}
	got, _ := srv.store.Get(sess.ID)
	h := got.ChatHistory()
	last := h[len(h)-1]
	if last.Role != "user" {
		t.Fatalf("recorded message role: want user, got %q", last.Role)
	}
	if !strings.Contains(last.Content, "$ echo hi") || !strings.Contains(last.Content, "hi") {
		t.Fatalf("history should record command + output, got %q", last.Content)
	}
}

// TestSessionShellRequiresCwd asserts a shell call on a session with no cwd is
// rejected, since a shell command outside a working directory is meaningless.
func TestSessionShellRequiresCwd(t *testing.T) {
	srv, client := newSetCwdServer(t, "/tmp/inari-test-shell-nocwd.sock")
	defer client.Close()

	sess := session.New("test") // no cwd set
	srv.store.Add(sess)

	if _, err := client.Shell(sess.ID, "echo hi"); err == nil {
		t.Fatal("expected an error running shell without a cwd, got nil")
	}
}

// tail returns the last n bytes of s for error messages.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
