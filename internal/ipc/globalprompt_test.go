package ipc

import (
	"os"
	"strings"
	"testing"

	"github.com/mirageglobe/inari/internal/audit"
	"github.com/mirageglobe/inari/internal/session"
)

// TestSessionCreateGlobalPrompt asserts the configured global system prompt is
// prepended to a new session's prompt while its base (default) prompt is retained.
func TestSessionCreateGlobalPrompt(t *testing.T) {
	auditFile, err := os.CreateTemp("", "inari-audit-*.log")
	if err != nil {
		t.Fatal(err)
	}
	auditFile.Close()
	t.Cleanup(func() { os.Remove(auditFile.Name()) })
	auditor := audit.New(auditFile.Name())
	t.Cleanup(func() { auditor.Close() })

	store := session.NewStore()
	sock := "/tmp/inari-test-globalprompt.sock"
	srv, err := NewServer(ServerConfig{
		Socket:             sock,
		Store:              store,
		Auditor:            auditor,
		Provider:           &fakeAssignProvider{},
		GlobalSystemPrompt: "GLOBAL RULES: be terse.",
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { srv.Close(); os.Remove(sock) })

	client := NewClient(sock)
	defer client.Close()

	info, err := client.CreateSession("s", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, _ := store.Get(info.ID)
	if !strings.HasPrefix(got.SystemPrompt, "GLOBAL RULES: be terse.") {
		t.Fatalf("system prompt should start with the global prompt, got %q", got.SystemPrompt)
	}
	if !strings.Contains(got.SystemPrompt, "concise") {
		t.Fatalf("base (default) prompt should be retained after the global prefix, got %q", got.SystemPrompt)
	}
}
