package ipc

import (
	"strings"
	"testing"

	"github.com/mirageglobe/inari/internal/provider"
)

// TestStreamShortCircuitsLowEffort asserts that low-effort input (no alphanumeric
// content) gets a canned local reply without invoking the model, and that the
// canned reply is persisted.
func TestStreamShortCircuitsLowEffort(t *testing.T) {
	// the fake would stream "hi"; a short-circuit means it is never called.
	fake := &fakeStreamProvider{running: []provider.RunningModel{{Name: "gemma4:e2b"}}}
	_, sess := newStreamTestServer(t, "/tmp/inari-test-presend.sock", fake)

	client := NewClient("/tmp/inari-test-presend.sock")
	defer client.Close()

	tokens := make(chan string, 8)
	statuses := make(chan string, 8)
	toolReqs := make(chan ToolRequestMsg, 1)
	approvals := make(chan bool, 1)

	if err := client.ChatStream(sess.ID, "???", tokens, statuses, toolReqs, approvals); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	close(tokens)
	var got string
	for tk := range tokens {
		got += tk
	}
	if strings.Contains(got, "hi") || !strings.Contains(got, "rephrase") {
		t.Fatalf("expected canned rephrase reply (model not called) for low-effort input, got %q", got)
	}

	hist := sess.ChatHistory()
	last := hist[len(hist)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "rephrase") {
		t.Fatalf("expected canned reply persisted, got %+v", last)
	}
}
