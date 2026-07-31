package ipc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirageglobe/inari/internal/audit"
	"github.com/mirageglobe/inari/internal/session"
)

// newProjectTestServer spins up a daemon with the given global prompt and returns
// a connected client, mirroring globalprompt_test's setup.
func newProjectTestServer(t *testing.T, sock, globalPrompt string) (*Server, *Client, *session.Store) {
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
	srv, err := NewServer(ServerConfig{
		Socket:             sock,
		Store:              store,
		Auditor:            auditor,
		Provider:           &fakeAssignProvider{},
		GlobalSystemPrompt: globalPrompt,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { srv.Close(); os.Remove(sock) })
	client := NewClient(sock)
	t.Cleanup(func() { client.Close() })
	return srv, client, store
}

// writeProjectConfig writes a .inari/config.json under dir.
func writeProjectConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".inari"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".inari", "config.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestSessionCreateProjectPromptOverridesGlobal asserts a project overlay's
// context.system_prompt replaces the global prompt for a session in that cwd,
// while the base cwd context is still retained.
func TestSessionCreateProjectPromptOverridesGlobal(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `{"context": {"system_prompt": "PROJECT RULES: cite files."}}`)

	_, client, store := newProjectTestServer(t, "/tmp/inari-test-projprompt.sock", "GLOBAL RULES: be terse.")
	info, err := client.CreateSession("s", dir)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, _ := store.Get(info.ID)
	if !strings.HasPrefix(got.SystemPrompt, "PROJECT RULES: cite files.") {
		t.Fatalf("project prompt should win over global, got %q", got.SystemPrompt)
	}
	if strings.Contains(got.SystemPrompt, "GLOBAL RULES") {
		t.Fatalf("global prompt should be replaced, not composed, got %q", got.SystemPrompt)
	}
	if !strings.Contains(got.SystemPrompt, "working directory:") {
		t.Fatalf("base cwd context should be retained, got %q", got.SystemPrompt)
	}
}

// TestSessionCreateProjectExcludeDirs asserts the overlay's exclude_dirs prune the
// named directory from the injected file tree.
func TestSessionCreateProjectExcludeDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "secret"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	writeProjectConfig(t, dir, `{"exclude_dirs": ["secret"]}`)

	_, client, store := newProjectTestServer(t, "/tmp/inari-test-projexclude.sock", "")
	info, err := client.CreateSession("s", dir)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, _ := store.Get(info.ID)
	if strings.Contains(got.SystemPrompt, "secret/") {
		t.Fatalf("excluded dir should be pruned from tree, got %q", got.SystemPrompt)
	}
	if !strings.Contains(got.SystemPrompt, "src/") {
		t.Fatalf("non-excluded dir should still appear, got %q", got.SystemPrompt)
	}
}

// TestSessionCreateGlobalPromptWithoutOverlay asserts the global prompt still
// applies when the cwd has no overlay (no regression).
func TestSessionCreateGlobalPromptWithoutOverlay(t *testing.T) {
	dir := t.TempDir()
	_, client, store := newProjectTestServer(t, "/tmp/inari-test-noverlay.sock", "GLOBAL RULES: be terse.")
	info, err := client.CreateSession("s", dir)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, _ := store.Get(info.ID)
	if !strings.HasPrefix(got.SystemPrompt, "GLOBAL RULES: be terse.") {
		t.Fatalf("global prompt should apply without an overlay, got %q", got.SystemPrompt)
	}
}
