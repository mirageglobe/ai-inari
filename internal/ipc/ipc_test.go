package ipc

import (
	"os"
	"testing"

	"github.com/mirageglobe/ai-sudama/internal/audit"
	"github.com/mirageglobe/ai-sudama/internal/mcp"
	"github.com/mirageglobe/ai-sudama/internal/scheduler"
	"github.com/mirageglobe/ai-sudama/internal/session"
)

func TestPingPong(t *testing.T) {
	sock := "/tmp/sudama-test.sock"
	defer os.Remove(sock)

	auditFile, err := os.CreateTemp("", "sudama-audit-*.log")
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

	srv, err := NewServer(sock, store, sched, host, auditor)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	client, err := NewClient(sock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	if err := client.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestSessionList(t *testing.T) {
	sock := "/tmp/sudama-test-list.sock"
	defer os.Remove(sock)

	auditFile, err := os.CreateTemp("", "sudama-audit-*.log")
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

	srv, err := NewServer(sock, store, sched, host, auditor)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	client, err := NewClient(sock)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	resp, err := client.Call("session.list", nil)
	if err != nil {
		t.Fatalf("Call session.list: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("session.list error: %s", resp.Error.Message)
	}
}
