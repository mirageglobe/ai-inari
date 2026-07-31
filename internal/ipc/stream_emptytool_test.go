package ipc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mirageglobe/inari/internal/provider"
)

// fakeEmptyAfterToolProvider returns a native list_dir tool_call on the first
// round, then an empty final answer, reproducing the gemma4:e2b behaviour where a
// model says nothing after a tool result.
type fakeEmptyAfterToolProvider struct {
	*fakeStreamProvider
	calls atomic.Int32
}

func (f *fakeEmptyAfterToolProvider) ChatStream(_ context.Context, _ provider.ChatRequest, chunks chan<- provider.ChatResponse) error {
	if f.calls.Add(1) == 1 {
		chunks <- provider.ChatResponse{Message: provider.Message{
			Role: "assistant",
			ToolCalls: []provider.ToolCall{{Function: provider.ToolCallFunction{
				Name: "list_dir", Arguments: map[string]any{"path": "."}}}},
		}}
		return nil
	}
	// subsequent rounds: empty content, no tool_calls (the model has nothing to say).
	chunks <- provider.ChatResponse{Message: provider.Message{Role: "assistant", Content: ""}}
	return nil
}

// TestStreamSurfacesToolOutputOnEmptyFinal asserts that when the model runs a tool
// then returns an empty final answer, inarid surfaces the tool result to the user
// instead of a blank reply, and persists it to history.
func TestStreamSurfacesToolOutputOnEmptyFinal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	fake := &fakeEmptyAfterToolProvider{
		fakeStreamProvider: &fakeStreamProvider{running: []provider.RunningModel{{Name: "gemma4:e2b"}}},
	}
	_, sess := newStreamTestServer(t, "/tmp/inari-test-empty-tool.sock", fake)
	sess.CWD = dir // enables builtin tools + sandboxes list_dir to this dir

	client := NewClient("/tmp/inari-test-empty-tool.sock")
	defer client.Close()

	tokens := make(chan string, 16)
	statuses := make(chan string, 8)
	toolReqs := make(chan ToolRequestMsg, 1)
	approvals := make(chan bool, 1)

	if err := client.ChatStream(sess.ID, "list the files", tokens, statuses, toolReqs, approvals); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	close(tokens)
	var got string
	for tk := range tokens {
		got += tk
	}
	if !strings.Contains(got, "marker.txt") {
		t.Fatalf("expected tool output surfaced to user, got %q", got)
	}

	hist := sess.ChatHistory()
	last := hist[len(hist)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "marker.txt") {
		t.Fatalf("expected tool output persisted as assistant reply, got %+v", last)
	}
}
