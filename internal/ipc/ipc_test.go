package ipc

import (
	"os"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/mcp"
	"github.com/mirageglobe/ai-inari/internal/provider"
	"github.com/mirageglobe/ai-inari/internal/scheduler"
	"github.com/mirageglobe/ai-inari/internal/session"
)

// fakeAssignProvider implements provider.Provider with only ListModels exercised;
// every other method is a stub since session.assign never calls them.
type fakeAssignProvider struct {
	models []provider.Model
}

func (f *fakeAssignProvider) Ping() error                                     { return nil }
func (f *fakeAssignProvider) Chat(string, []provider.Message) (string, error) { return "", nil }
func (f *fakeAssignProvider) ChatStream(provider.ChatRequest, chan<- provider.ChatResponse) error {
	return nil
}
func (f *fakeAssignProvider) LoadModel(string) error   { return nil }
func (f *fakeAssignProvider) UnloadModel(string) error { return nil }
func (f *fakeAssignProvider) ListModels() ([]provider.Model, error) {
	return f.models, nil
}
func (f *fakeAssignProvider) ListRunning() ([]provider.RunningModel, error) { return nil, nil }
func (f *fakeAssignProvider) ModelCaps(string) ([]string, error)            { return nil, nil }
func (f *fakeAssignProvider) PullModel(string, chan<- provider.PullProgress) error {
	return nil
}

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

	srv, err := NewServer(sock, store, sched, host, auditor, nil, false, 0, "")
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

func TestModelFor(t *testing.T) {
	s := &Server{defaultModel: "gemma4:e4b"}

	assigned := &session.Session{Model: "bonsai:4b"}
	if got := s.modelFor(assigned); got != "bonsai:4b" {
		t.Errorf("modelFor(assigned) = %q, want bonsai:4b", got)
	}

	unassigned := &session.Session{}
	if got := s.modelFor(unassigned); got != "gemma4:e4b" {
		t.Errorf("modelFor(unassigned) = %q, want gemma4:e4b (default fallback)", got)
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

	srv, err := NewServer(sock, store, sched, host, auditor, nil, false, 0, "")
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

func TestSessionAssign(t *testing.T) {
	newServer := func(t *testing.T, sock string, fake *fakeAssignProvider) (*Server, *Client) {
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

		srv, err := NewServer(sock, store, sched, host, auditor, fake, false, 0, "")
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		t.Cleanup(func() { srv.Close(); os.Remove(sock) })

		return srv, NewClient(sock)
	}

	t.Run("rejects a model not in the provider's list", func(t *testing.T) {
		fake := &fakeAssignProvider{models: []provider.Model{{Name: "gemma4:e2b"}}}
		srv, client := newServer(t, "/tmp/inari-test-assign-unknown.sock", fake)
		defer client.Close()

		sess := session.New("test")
		srv.store.Add(sess)

		if err := client.AssignModel(sess.ID, "deepseek-r1:7b"); err == nil {
			t.Fatal("expected an error for a model absent from ListModels, got nil")
		}

		got, _ := srv.store.Get(sess.ID)
		if got.Model != "" {
			t.Fatalf("expected model to remain unassigned, got %q", got.Model)
		}
	})

	t.Run("accepts a model present in the provider's list", func(t *testing.T) {
		fake := &fakeAssignProvider{models: []provider.Model{{Name: "gemma4:e2b"}}}
		srv, client := newServer(t, "/tmp/inari-test-assign-known.sock", fake)
		defer client.Close()

		sess := session.New("test")
		srv.store.Add(sess)

		if err := client.AssignModel(sess.ID, "gemma4:e2b"); err != nil {
			t.Fatalf("AssignModel: %v", err)
		}

		got, _ := srv.store.Get(sess.ID)
		if got.Model != "gemma4:e2b" {
			t.Fatalf("expected model gemma4:e2b, got %q", got.Model)
		}
	})
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
