package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mirageglobe/inari/internal/provider"
)

func TestChatReturnsMessageContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"message":{"role":"assistant","content":"hello"},"done":true}`)
	}))
	defer srv.Close()

	reply, err := NewClient(srv.URL).Chat("gemma4:e4b", []provider.Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if reply != "hello" {
		t.Errorf("got %q, want %q", reply, "hello")
	}
}

func TestChatPropagatesErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"bad request"}`)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).Chat("gemma4:e4b", nil)
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
}

func TestChatStreamForwardsChunksUntilDone(t *testing.T) {
	lines := []string{
		`{"message":{"role":"assistant","content":"hel"},"done":false}`,
		`{"message":{"role":"assistant","content":"lo"},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
	}))
	defer srv.Close()

	out := make(chan provider.ChatResponse, 8)
	req := provider.ChatRequest{Model: "gemma4:e4b", Messages: []provider.Message{{Role: "user", Content: "hi"}}}
	if err := NewClient(srv.URL).ChatStream(context.Background(), req, out); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	close(out)

	var got []provider.ChatResponse
	for c := range out {
		got = append(got, c)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %+v", len(got), got)
	}
	if !got[2].Done {
		t.Errorf("final chunk Done = false, want true")
	}
}

func TestChatStreamPropagatesErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":"overloaded"}`)
	}))
	defer srv.Close()

	out := make(chan provider.ChatResponse, 8)
	req := provider.ChatRequest{Model: "gemma4:e4b"}
	if err := NewClient(srv.URL).ChatStream(context.Background(), req, out); err == nil {
		t.Fatal("expected error for 503 response, got nil")
	}
}
