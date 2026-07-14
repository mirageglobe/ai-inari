package ipc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/mcp"
	"github.com/mirageglobe/ai-inari/internal/scheduler"
	"github.com/mirageglobe/ai-inari/internal/session"
)

// newSetCwdServer builds a minimal server + client for session.setcwd tests.
func newSetCwdServer(t *testing.T, sock string) (*Server, *Client) {
	t.Helper()
	auditFile, err := os.CreateTemp("", "inari-audit-*.log")
	if err != nil {
		t.Fatal(err)
	}
	auditFile.Close()
	t.Cleanup(func() { os.Remove(auditFile.Name()) })

	auditor := audit.New(auditFile.Name())
	t.Cleanup(func() { auditor.Close() })

	store := session.NewStore()
	sched := scheduler.New(8192)
	host := mcp.NewHost(nil, auditor)

	srv, err := NewServer(sock, store, sched, host, auditor, &fakeAssignProvider{}, false, 0, "", "")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { srv.Close(); os.Remove(sock) })
	return srv, NewClient(sock)
}

// TestSessionSetCwd covers the happy path (valid dir updates cwd + system prompt),
// rejection of a non-directory path, and an unknown session id.
func TestSessionSetCwd(t *testing.T) {
	t.Run("valid directory updates cwd and system prompt", func(t *testing.T) {
		srv, client := newSetCwdServer(t, "/tmp/inari-test-setcwd-ok.sock")
		defer client.Close()

		dir := t.TempDir()
		sess := session.New("test")
		srv.store.Add(sess)

		info, err := client.SetCwd(sess.ID, dir)
		if err != nil {
			t.Fatalf("SetCwd: %v", err)
		}
		if info.CWD != dir {
			t.Fatalf("returned cwd = %q, want %q", info.CWD, dir)
		}
		got, _ := srv.store.Get(sess.ID)
		if got.CWD != dir {
			t.Fatalf("stored cwd = %q, want %q", got.CWD, dir)
		}
		if !strings.Contains(got.SystemPrompt, "working directory: "+dir) {
			t.Fatalf("system prompt missing working-directory context; got %q", got.SystemPrompt)
		}
	})

	t.Run("rejects a path that is not a directory", func(t *testing.T) {
		srv, client := newSetCwdServer(t, "/tmp/inari-test-setcwd-bad.sock")
		defer client.Close()

		sess := session.New("test")
		srv.store.Add(sess)

		missing := filepath.Join(t.TempDir(), "does-not-exist")
		if _, err := client.SetCwd(sess.ID, missing); err == nil {
			t.Fatal("expected an error for a non-directory path, got nil")
		}
		got, _ := srv.store.Get(sess.ID)
		if got.CWD != "" {
			t.Fatalf("cwd should be unchanged on error, got %q", got.CWD)
		}
	})

	t.Run("rejects an unknown session id", func(t *testing.T) {
		_, client := newSetCwdServer(t, "/tmp/inari-test-setcwd-nosess.sock")
		defer client.Close()

		if _, err := client.SetCwd("deadbeef", t.TempDir()); err == nil {
			t.Fatal("expected an error for an unknown session, got nil")
		}
	})
}
