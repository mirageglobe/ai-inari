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

// fakeStreamProvider implements provider.Provider with only ChatStream and
// ListRunning exercised; every other method is a stub since handleStream never
// calls them.
type fakeStreamProvider struct {
	running []provider.RunningModel
}

func (f *fakeStreamProvider) Ping() error                                     { return nil }
func (f *fakeStreamProvider) Chat(string, []provider.Message) (string, error) { return "", nil }
func (f *fakeStreamProvider) ChatStream(_ context.Context, req provider.ChatRequest, chunks chan<- provider.ChatResponse) error {
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

// fakeInterruptProvider streams one chunk then blocks until ctx is cancelled,
// so a test can drive the interrupt path deterministically. it embeds
// fakeStreamProvider for the stub methods and overrides ChatStream.
type fakeInterruptProvider struct {
	*fakeStreamProvider
	started chan struct{} // closed once the first chunk has been sent
}

func (f *fakeInterruptProvider) ChatStream(ctx context.Context, _ provider.ChatRequest, chunks chan<- provider.ChatResponse) error {
	chunks <- provider.ChatResponse{Message: provider.Message{Role: "assistant", Content: "partial reply"}}
	close(f.started)
	<-ctx.Done() // block until interrupted
	return ctx.Err()
}

// TestStreamInterruptKeepsPartialReply asserts that a session.interrupt RPC
// cancels an in-flight stream, the stream ends cleanly (done, not error), and the
// partial reply generated so far is forwarded and persisted to history.
func TestStreamInterruptKeepsPartialReply(t *testing.T) {
	fake := &fakeInterruptProvider{
		fakeStreamProvider: &fakeStreamProvider{running: []provider.RunningModel{{Name: "gemma4:e2b"}}},
		started:            make(chan struct{}),
	}
	_, sess := newStreamTestServer(t, "/tmp/inari-test-stream-interrupt.sock", fake)

	streamClient := NewClient("/tmp/inari-test-stream-interrupt.sock")
	defer streamClient.Close()

	tokens := make(chan string, 8)
	statuses := make(chan string, 8)
	toolReqs := make(chan ToolRequestMsg, 1)
	approvals := make(chan bool, 1)

	errc := make(chan error, 1)
	go func() {
		errc <- streamClient.ChatStream(sess.ID, "hello", tokens, statuses, toolReqs, approvals)
	}()

	<-fake.started // wait until the first chunk is streamed and the provider blocks

	// interrupt over a separate connection, mirroring how inari's shared client
	// issues the RPC independently of the dedicated stream connection.
	ctlClient := NewClient("/tmp/inari-test-stream-interrupt.sock")
	defer ctlClient.Close()
	if err := ctlClient.Interrupt(sess.ID); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	if err := <-errc; err != nil {
		t.Fatalf("ChatStream after interrupt should end cleanly, got: %v", err)
	}
	close(tokens)
	var got string
	for tk := range tokens {
		got += tk
	}
	if got != "partial reply" {
		t.Fatalf("expected partial token forwarded, got %q", got)
	}

	hist := sess.ChatHistory()
	last := hist[len(hist)-1]
	if last.Role != "assistant" || last.Content != "partial reply" {
		t.Fatalf("expected assistant partial reply persisted, got %+v", last)
	}
}

// TestInterruptNoActiveStream asserts session.interrupt reports interrupted=false
// when the session has no stream in flight.
func TestInterruptNoActiveStream(t *testing.T) {
	fake := &fakeStreamProvider{}
	_, sess := newStreamTestServer(t, "/tmp/inari-test-interrupt-none.sock", fake)

	client := NewClient("/tmp/inari-test-interrupt-none.sock")
	defer client.Close()

	resp, err := client.Call("session.interrupt", map[string]string{"id": sess.ID})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type %T", resp.Result)
	}
	if m["interrupted"] != false {
		t.Fatalf("expected interrupted=false, got %v", m["interrupted"])
	}
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
