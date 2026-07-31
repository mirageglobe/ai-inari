package ipc

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/provider"
	"github.com/mirageglobe/ai-inari/internal/session"
)

// fakeMetricsProvider streams one content chunk then a final chunk carrying the
// inference counters, which is the shape ollama actually sends.
type fakeMetricsProvider struct{ *fakeStreamProvider }

func (f *fakeMetricsProvider) ChatStream(_ context.Context, _ provider.ChatRequest, chunks chan<- provider.ChatResponse) error {
	chunks <- provider.ChatResponse{Message: provider.Message{Role: "assistant", Content: "hi"}}
	chunks <- provider.ChatResponse{
		Done:               true,
		TotalDuration:      4883583458,
		LoadDuration:       1334875,
		PromptEvalCount:    26,
		PromptEvalDuration: 342546000,
		EvalCount:          282,
		EvalDuration:       4535599000,
	}
	return nil
}

// newMetricsTestServer wires a server against p with its own audit log and returns
// the log path so a test can assert on what was written.
func newMetricsTestServer(t *testing.T, sock string, p provider.Provider) (*session.Session, string) {
	t.Helper()
	os.Remove(sock)
	t.Cleanup(func() { os.Remove(sock) })

	auditFile, err := os.CreateTemp("", "inari-audit-*.log")
	if err != nil {
		t.Fatal(err)
	}
	auditPath := auditFile.Name()
	auditFile.Close()
	t.Cleanup(func() { os.Remove(auditPath) })

	auditor := audit.New(auditPath)
	t.Cleanup(func() { auditor.Close() })

	store := session.NewStore()
	srv, err := NewServer(ServerConfig{Socket: sock, Store: store, Auditor: auditor, Provider: p})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	sess := session.New("test")
	sess.Model = "gemma4:e2b"
	sess.CWD = t.TempDir()
	store.Add(sess)
	return sess, auditPath
}

// drainTurn runs one full turn against sock so the audit log is complete.
func drainTurn(t *testing.T, sock, sessionID string) {
	t.Helper()
	client := NewClient(sock)
	defer client.Close()

	tokens := make(chan string, 16)
	statuses := make(chan string, 8)
	toolReqs := make(chan ToolRequestMsg, 1)
	approvals := make(chan bool, 1)

	if err := client.ChatStream(sessionID, "hi", tokens, statuses, toolReqs, approvals); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	close(tokens)
	for range tokens {
	}
}

// TestStreamAuditsTurnMetrics asserts the counters ollama returns on its final
// chunk reach the audit log as their own turn.metrics entry. without this the
// numbers are decoded and then dropped, which is the state the roadmap item
// describes.
func TestStreamAuditsTurnMetrics(t *testing.T) {
	sock := "/tmp/inari-test-metrics.sock"
	fake := &fakeMetricsProvider{
		fakeStreamProvider: &fakeStreamProvider{running: []provider.RunningModel{{Name: "gemma4:e2b"}}},
	}
	sess, auditPath := newMetricsTestServer(t, sock, fake)
	drainTurn(t, sock, sess.ID)

	logged, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(logged)
	for _, want := range []string{
		`"method":"turn.metrics"`,
		`"model":"gemma4:e2b"`,
		`"eval_tokens":282`,
		`"prompt_tokens":26`,
		`"tokens_per_sec":62.17`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("audit log missing %s, got: %s", want, got)
		}
	}
}

// a backend that reports no counters must not produce an all-zero record; an
// empty metrics line would read as "this turn generated nothing at zero tok/s".
func TestStreamSkipsMetricsWithoutCounters(t *testing.T) {
	sock := "/tmp/inari-test-metrics-none.sock"
	fake := &fakeStreamProvider{running: []provider.RunningModel{{Name: "gemma4:e2b"}}}
	sess, auditPath := newMetricsTestServer(t, sock, fake)
	drainTurn(t, sock, sess.ID)

	logged, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logged), "turn.metrics") {
		t.Errorf("expected no turn.metrics entry when the backend reports no counters, got: %s", logged)
	}
}
