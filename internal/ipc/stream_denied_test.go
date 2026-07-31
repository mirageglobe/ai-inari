package ipc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mirageglobe/inari/internal/audit"
	"github.com/mirageglobe/inari/internal/provider"
	"github.com/mirageglobe/inari/internal/session"
)

// fakeDeniedToolProvider asks for an unlisted shell binary on the first round, so
// the call misses the auto-approve gate and reaches the user-approval path, then
// returns a plain answer once the denial comes back.
type fakeDeniedToolProvider struct {
	*fakeStreamProvider
	calls atomic.Int32
}

func (f *fakeDeniedToolProvider) ChatStream(_ context.Context, _ provider.ChatRequest, chunks chan<- provider.ChatResponse) error {
	if f.calls.Add(1) == 1 {
		chunks <- provider.ChatResponse{Message: provider.Message{
			Role: "assistant",
			ToolCalls: []provider.ToolCall{{Function: provider.ToolCallFunction{
				Name:      "execute_shell_command",
				Arguments: map[string]any{"command": "curl", "args": "http://example.com"}}}},
		}}
		return nil
	}
	chunks <- provider.ChatResponse{Message: provider.Message{Role: "assistant", Content: "understood"}}
	return nil
}

// §8.2 layer C says rejection is logged and §5.1 calls the audit log a record of
// all tool calls. a denied call used to leave no trace on disk at all, so a
// rejected curl was invisible to exactly the trail you would check to find what
// the model tried to do.
func TestStreamAuditsDeniedToolCall(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	sock := "/tmp/inari-test-audit-denied.sock"
	os.Remove(sock)
	t.Cleanup(func() { os.Remove(sock) })

	auditFile, err := os.CreateTemp("", "inari-audit-denied-*.log")
	if err != nil {
		t.Fatal(err)
	}
	auditPath := auditFile.Name()
	auditFile.Close()
	t.Cleanup(func() { os.Remove(auditPath) })

	auditor := audit.New(auditPath)
	t.Cleanup(func() { auditor.Close() })

	store := session.NewStore()
	fake := &fakeDeniedToolProvider{
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
	go func() {
		for range toolReqs {
			approvals <- false // deny: this is the path under test
		}
	}()

	if err := client.ChatStream(sess.ID, "fetch that page", tokens, statuses, toolReqs, approvals); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	close(tokens)
	close(toolReqs)
	for range tokens {
	}

	auditor.Close()
	logged, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	if !strings.Contains(got, `"method":"tool.denied"`) || !strings.Contains(got, `"tool":"execute_shell_command"`) {
		t.Fatalf("expected a tool.denied audit entry, got: %s", got)
	}
	// the args are the point of the record: "the model tried to curl this url".
	if !strings.Contains(got, "example.com") {
		t.Errorf("denied entry should carry the args, got: %s", got)
	}
	// a denied call must not also be recorded as executed.
	if strings.Contains(got, `"method":"tool.call"`) {
		t.Errorf("denied call must not be logged as tool.call, got: %s", got)
	}
}
