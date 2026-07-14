package ipc

import (
	"context"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/provider"
	"github.com/mirageglobe/ai-inari/internal/session"
)

// TestSessionTag covers toggle-on, toggle-off, and empty-tag rejection.
func TestSessionTag(t *testing.T) {
	srv, client := newSetCwdServer(t, "/tmp/inari-test-tag.sock")
	defer client.Close()

	sess := session.New("s")
	srv.store.Add(sess)

	info, err := client.Tag(sess.ID, "work")
	if err != nil {
		t.Fatalf("Tag: %v", err)
	}
	if len(info.Tags) != 1 || info.Tags[0] != "work" {
		t.Fatalf("expected [work], got %v", info.Tags)
	}

	// toggling the same label removes it
	info, err = client.Tag(sess.ID, "work")
	if err != nil {
		t.Fatalf("Tag toggle: %v", err)
	}
	if len(info.Tags) != 0 {
		t.Fatalf("expected tag removed, got %v", info.Tags)
	}

	if _, err := client.Tag(sess.ID, ""); err == nil {
		t.Fatal("expected an error for an empty tag, got nil")
	}
}

// TestSessionSetNumCtx covers set, clear (0), negative clamp, and unknown session.
func TestSessionSetNumCtx(t *testing.T) {
	srv, client := newSetCwdServer(t, "/tmp/inari-test-setnumctx.sock")
	defer client.Close()

	sess := session.New("s")
	srv.store.Add(sess)

	info, err := client.SetNumCtx(sess.ID, 4096)
	if err != nil {
		t.Fatalf("SetNumCtx: %v", err)
	}
	if info.NumCtxOverride != 4096 {
		t.Fatalf("returned override = %d, want 4096", info.NumCtxOverride)
	}
	if got, _ := srv.store.Get(sess.ID); got.NumCtxOverride != 4096 {
		t.Fatalf("stored override = %d, want 4096", got.NumCtxOverride)
	}

	if info, _ = client.SetNumCtx(sess.ID, 0); info.NumCtxOverride != 0 {
		t.Fatalf("0 should clear override, got %d", info.NumCtxOverride)
	}
	if info, _ = client.SetNumCtx(sess.ID, -5); info.NumCtxOverride != 0 {
		t.Fatalf("negative should clamp to 0, got %d", info.NumCtxOverride)
	}

	if _, err := client.SetNumCtx("deadbeef", 100); err == nil {
		t.Fatal("expected an error for an unknown session, got nil")
	}
}

// recordingProvider captures the ChatRequest options handleStream sends so a test
// can assert the per-session num_ctx override is applied. embeds fakeStreamProvider.
type recordingProvider struct {
	*fakeStreamProvider
	lastOpts map[string]any
}

func (r *recordingProvider) ChatStream(_ context.Context, req provider.ChatRequest, chunks chan<- provider.ChatResponse) error {
	r.lastOpts = req.Options
	chunks <- provider.ChatResponse{Message: provider.Message{Role: "assistant", Content: "hi"}}
	return nil
}

// TestStreamUsesNumCtxOverride asserts handleStream requests the session's num_ctx
// override even when the model's own context length is unknown (fake returns 0).
func TestStreamUsesNumCtxOverride(t *testing.T) {
	rec := &recordingProvider{fakeStreamProvider: &fakeStreamProvider{running: []provider.RunningModel{{Name: "gemma4:e2b"}}}}
	_, sess := newStreamTestServer(t, "/tmp/inari-test-numctx-stream.sock", rec)
	sess.SetNumCtx(4096)

	client := NewClient("/tmp/inari-test-numctx-stream.sock")
	defer client.Close()

	tokens := make(chan string, 8)
	statuses := make(chan string, 8)
	toolReqs := make(chan ToolRequestMsg, 1)
	approvals := make(chan bool, 1)

	if err := client.ChatStream(sess.ID, "hi", tokens, statuses, toolReqs, approvals); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if rec.lastOpts["num_ctx"] != 4096 {
		t.Fatalf("expected num_ctx 4096 from override, got %v", rec.lastOpts["num_ctx"])
	}
}
