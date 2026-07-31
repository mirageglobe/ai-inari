package session

import (
	"testing"

	"github.com/mirageglobe/inari/internal/provider"
)

// TestRemoveLast asserts RemoveLast drops exactly the final message.
func TestRemoveLast(t *testing.T) {
	s := New("t")
	base := len(s.ChatHistory()) // system prompt seeded by New
	s.AppendMessage(provider.Message{Role: "user", Content: "a"})
	s.AppendMessage(provider.Message{Role: "assistant", Content: "b"})

	s.RemoveLast()
	h := s.ChatHistory()
	if len(h) != base+1 {
		t.Fatalf("expected %d messages after RemoveLast, got %d", base+1, len(h))
	}
	if h[len(h)-1].Content != "a" {
		t.Fatalf("RemoveLast should drop the last message; last is %q", h[len(h)-1].Content)
	}
}

// TestRemoveLastEmptyNoop asserts RemoveLast is a safe no-op on empty history.
func TestRemoveLastEmptyNoop(t *testing.T) {
	s := New("t")
	for len(s.ChatHistory()) > 0 {
		s.RemoveLast()
	}
	s.RemoveLast() // must not panic on empty
	if len(s.ChatHistory()) != 0 {
		t.Fatal("RemoveLast on empty history should be a no-op")
	}
}
