package ipc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/provider"
	"github.com/mirageglobe/ai-inari/internal/session"
)

// TestStreamAuditsToolCall asserts that a model-invoked tool call is written to
// the audit log as its own "tool.call" entry (name + args), not just the outer
// session.stream request that started the turn. reuses fakeEmptyAfterToolProvider
// (list_dir on round 1, empty final answer) from stream_emptytool_test.go.
func TestStreamAuditsToolCall(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	sock := "/tmp/inari-test-audit-tool.sock"
	os.Remove(sock)
	t.Cleanup(func() { os.Remove(sock) })

	auditFile, err := os.CreateTemp("", "inari-audit-*.log")
	if err != nil {
		t.Fatal(err)
	}
	auditPath := auditFile.Name()
	auditFile.Close()
	t.Cleanup(func() { os.Remove(auditPath) })

	auditor := audit.New(auditPath)
	t.Cleanup(func() { auditor.Close() })

	store := session.NewStore()
	fake := &fakeEmptyAfterToolProvider{
		fakeStreamProvider: &fakeStreamProvider{running: []provider.RunningModel{{Name: "gemma4:e2b"}}},
	}

	srv, err := NewServer(ServerConfig{Socket: sock, Store: store, Auditor: auditor, Provider: fake})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	sess := session.New("test")
	sess.Model = "gemma4:e2b"
	sess.CWD = dir
	store.Add(sess)

	client := NewClient(sock)
	defer client.Close()

	tokens := make(chan string, 16)
	statuses := make(chan string, 8)
	toolReqs := make(chan ToolRequestMsg, 1)
	approvals := make(chan bool, 1)

	if err := client.ChatStream(sess.ID, "list the files", tokens, statuses, toolReqs, approvals); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	close(tokens)
	for range tokens {
	}

	auditor.Close()
	logged, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), `"method":"tool.call"`) || !strings.Contains(string(logged), `"tool":"list_dir"`) {
		t.Fatalf("expected a tool.call audit entry for list_dir, got: %s", logged)
	}
}
