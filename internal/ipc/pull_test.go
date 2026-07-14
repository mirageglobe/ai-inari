package ipc

import (
	"context"
	"os"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/mcp"
	"github.com/mirageglobe/ai-inari/internal/provider"
	"github.com/mirageglobe/ai-inari/internal/scheduler"
	"github.com/mirageglobe/ai-inari/internal/session"
)

// fakePullProvider implements provider.Provider with only PullModel exercised;
// every other method is a stub since model.pull never calls them.
type fakePullProvider struct {
	updates []provider.PullProgress
	err     error
}

func (f *fakePullProvider) Ping() error                                     { return nil }
func (f *fakePullProvider) Chat(string, []provider.Message) (string, error) { return "", nil }
func (f *fakePullProvider) ChatStream(context.Context, provider.ChatRequest, chan<- provider.ChatResponse) error {
	return nil
}
func (f *fakePullProvider) LoadModel(string) error                        { return nil }
func (f *fakePullProvider) UnloadModel(string) error                      { return nil }
func (f *fakePullProvider) ListModels() ([]provider.Model, error)         { return nil, nil }
func (f *fakePullProvider) ListRunning() ([]provider.RunningModel, error) { return nil, nil }
func (f *fakePullProvider) ModelCaps(string) ([]string, error)            { return nil, nil }
func (f *fakePullProvider) PullModel(model string, out chan<- provider.PullProgress) error {
	for _, u := range f.updates {
		out <- u
	}
	return f.err
}
func (f *fakePullProvider) DeleteModel(string) error               { return nil }
func (f *fakePullProvider) ModelContextLength(string) (int, error) { return 0, nil }

func TestModelPullStreamsProgress(t *testing.T) {
	sock := "/tmp/inari-test-pull.sock"
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

	fake := &fakePullProvider{updates: []provider.PullProgress{
		{Status: "pulling manifest"},
		{Status: "downloading", Completed: 50, Total: 100},
		{Status: "success"},
	}}

	srv, err := NewServer(sock, store, sched, host, auditor, fake, false, 0, "", "")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	client := NewClient(sock)
	defer client.Close()

	progress := make(chan provider.PullProgress, 8)
	if err := client.PullModel("gemma4:e4b", progress); err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	close(progress)

	var got []provider.PullProgress
	for p := range progress {
		got = append(got, p)
	}
	if len(got) != 3 || got[2].Status != "success" {
		t.Fatalf("expected 3 updates ending in success, got %+v", got)
	}
}

func TestModelPullMissingModel(t *testing.T) {
	sock := "/tmp/inari-test-pull-missing.sock"
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

	srv, err := NewServer(sock, store, sched, host, auditor, nil, false, 0, "", "")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	client := NewClient(sock)
	defer client.Close()

	progress := make(chan provider.PullProgress, 1)
	if err := client.PullModel("", progress); err == nil {
		t.Fatal("expected error for empty model name, got nil")
	}
}
