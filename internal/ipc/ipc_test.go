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

	srv, err := NewServer(sock, store, sched, host, auditor, nil)
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

	srv, err := NewServer(sock, store, sched, host, auditor, nil)
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
