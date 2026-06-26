package ipc

import (
	"os"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/mcp"
	"github.com/mirageglobe/ai-inari/internal/scheduler"
	"github.com/mirageglobe/ai-inari/internal/session"
)

func TestPingPong(t *testing.T) {
	sock := "/tmp/inari-test.sock"
	defer os.Remove(sock)

	auditFile, err := os.CreateTemp("", "inari-audit-*.log")
	if err != nil {
		t.Fatal(err)
	}
	auditFile.Close()
	defer os.Remove(auditFile.Name())

	auditor := audit.New(auditFile.Name())
	defer auditor.Close()

	store := session.NewStore()
	sched := scheduler.New(8192)
	host := mcp.NewHost(nil, auditor)

	srv, err := NewServer(sock, store, sched, host, auditor, nil, false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	client := NewClient(sock)
	defer client.Close()

	if err := client.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestSessionList(t *testing.T) {
	sock := "/tmp/inari-test-list.sock"
	defer os.Remove(sock)

	auditFile, err := os.CreateTemp("", "inari-audit-*.log")
	if err != nil {
		t.Fatal(err)
	}
	auditFile.Close()
	defer os.Remove(auditFile.Name())

	auditor := audit.New(auditFile.Name())
	defer auditor.Close()

	store := session.NewStore()
	sched := scheduler.New(8192)
	host := mcp.NewHost(nil, auditor)

	srv, err := NewServer(sock, store, sched, host, auditor, nil, false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	client := NewClient(sock)
	defer client.Close()

	resp, err := client.Call("session.list", nil)
	if err != nil {
		t.Fatalf("Call session.list: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("session.list error: %s", resp.Error.Message)
	}
}

func TestReadAgentContext(t *testing.T) {
	t.Run("absent returns empty", func(t *testing.T) {
		if got := readAgentContext(t.TempDir()); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("reads AGENTS.md", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(dir+"/AGENTS.md", []byte("  use tabs  "), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := readAgentContext(dir); got != "use tabs" {
			t.Fatalf("expected trimmed content, got %q", got)
		}
	})

	t.Run("AGENTS.md takes priority over .inari/context.md", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(dir+"/.inari", 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir+"/.inari/context.md", []byte("fallback"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir+"/AGENTS.md", []byte("primary"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := readAgentContext(dir); got != "primary" {
			t.Fatalf("expected primary, got %q", got)
		}
	})

	t.Run("truncates to cap", func(t *testing.T) {
		dir := t.TempDir()
		big := make([]byte, agentContextCap+100)
		for i := range big {
			big[i] = 'x'
		}
		if err := os.WriteFile(dir+"/AGENTS.md", big, 0o600); err != nil {
			t.Fatal(err)
		}
		if got := readAgentContext(dir); len(got) != agentContextCap {
			t.Fatalf("expected len %d, got %d", agentContextCap, len(got))
		}
	})
}
