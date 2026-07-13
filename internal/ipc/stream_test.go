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

// fakeStreamProvider implements provider.Provider with only ChatStream and
// ListRunning exercised; every other method is a stub since handleStream never
// calls them.
type fakeStreamProvider struct {
	running []provider.RunningModel
}

func (f *fakeStreamProvider) Ping() error                                     { return nil }
func (f *fakeStreamProvider) Chat(string, []provider.Message) (string, error) { return "", nil }
func (f *fakeStreamProvider) ChatStream(req provider.ChatRequest, chunks chan<- provider.ChatResponse) error {
	chunks <- provider.ChatResponse{Message: provider.Message{Role: "assistant", Content: "hi"}}
	return nil
}
func (f *fakeStreamProvider) LoadModel(string) error                        { return nil }
func (f *fakeStreamProvider) UnloadModel(string) error                      { return nil }
func (f *fakeStreamProvider) ListModels() ([]provider.Model, error)         { return nil, nil }
func (f *fakeStreamProvider) ListRunning() ([]provider.RunningModel, error) { return f.running, nil }
func (f *fakeStreamProvider) ModelCaps(string) ([]string, error)            { return nil, nil }
func (f *fakeStreamProvider) PullModel(string, chan<- provider.PullProgress) error {
	return nil
}
func (f *fakeStreamProvider) DeleteModel(string) error               { return nil }
func (f *fakeStreamProvider) ModelContextLength(string) (int, error) { return 0, nil }

func newStreamTestServer(t *testing.T, sock string, p provider.Provider) (*Server, *session.Session) {
	t.Helper()
	os.Remove(sock)
	t.Cleanup(func() { os.Remove(sock) })

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

	srv, err := NewServer(sock, store, sched, host, auditor, p, false, 0, "")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	sess := session.New("test")
	sess.Model = "gemma4:e2b"
	store.Add(sess)
	return srv, sess
}

// TestStreamSignalsLoadingWhenModelNotResident asserts that handleStream emits a
// "loading" status followed by "thinking" when the assigned model is absent from
// the backend's currently-loaded models, since the next request will cold-load it.
func TestStreamSignalsLoadingWhenModelNotResident(t *testing.T) {
	fake := &fakeStreamProvider{running: nil}
	_, sess := newStreamTestServer(t, "/tmp/inari-test-stream-loading.sock", fake)

	client := NewClient("/tmp/inari-test-stream-loading.sock")
	defer client.Close()

	tokens := make(chan string, 8)
	statuses := make(chan string, 8)
	toolReqs := make(chan ToolRequestMsg, 1)
	approvals := make(chan bool, 1)

	if err := client.ChatStream(sess.ID, "hello", tokens, statuses, toolReqs, approvals); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	close(statuses)

	var got []string
	for s := range statuses {
		got = append(got, s)
	}
	if len(got) != 2 || got[0] != "loading" || got[1] != "thinking" {
		t.Fatalf("expected [loading thinking] status sequence, got %v", got)
	}
}

// TestStreamSkipsLoadingWhenModelResident asserts no "loading" status is sent
// when the assigned model is already resident in backend memory.
func TestStreamSkipsLoadingWhenModelResident(t *testing.T) {
	fake := &fakeStreamProvider{running: []provider.RunningModel{{Name: "gemma4:e2b"}}}
	_, sess := newStreamTestServer(t, "/tmp/inari-test-stream-resident.sock", fake)

	client := NewClient("/tmp/inari-test-stream-resident.sock")
	defer client.Close()

	tokens := make(chan string, 8)
	statuses := make(chan string, 8)
	toolReqs := make(chan ToolRequestMsg, 1)
	approvals := make(chan bool, 1)

	if err := client.ChatStream(sess.ID, "hello", tokens, statuses, toolReqs, approvals); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	close(statuses)

	for s := range statuses {
		t.Fatalf("expected no status frames when model is resident, got %q", s)
	}
}
