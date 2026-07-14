package ipc

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/mcp"
	"github.com/mirageglobe/ai-inari/internal/provider"
	"github.com/mirageglobe/ai-inari/internal/scheduler"
	"github.com/mirageglobe/ai-inari/internal/session"
)

// fakeRecapProvider returns a fixed recap from Chat; every other method is a stub.
type fakeRecapProvider struct{}

func (fakeRecapProvider) Ping() error { return nil }
func (fakeRecapProvider) Chat(string, []provider.Message) (string, error) {
	return "you were debugging the recap RPC.", nil
}
func (fakeRecapProvider) ChatStream(context.Context, provider.ChatRequest, chan<- provider.ChatResponse) error {
	return nil
}
func (fakeRecapProvider) LoadModel(string) error                               { return nil }
func (fakeRecapProvider) UnloadModel(string) error                             { return nil }
func (fakeRecapProvider) ListModels() ([]provider.Model, error)                { return nil, nil }
func (fakeRecapProvider) ListRunning() ([]provider.RunningModel, error)        { return nil, nil }
func (fakeRecapProvider) ModelCaps(string) ([]string, error)                   { return nil, nil }
func (fakeRecapProvider) PullModel(string, chan<- provider.PullProgress) error { return nil }
func (fakeRecapProvider) DeleteModel(string) error                             { return nil }
func (fakeRecapProvider) ModelContextLength(string) (int, error)               { return 0, nil }

func newRecapServer(t *testing.T, sock string) (*Server, *Client) {
	t.Helper()
	os.Remove(sock)
	af, err := os.CreateTemp("", "inari-audit-*.log")
	if err != nil {
		t.Fatal(err)
	}
	af.Close()
	t.Cleanup(func() { os.Remove(af.Name()) })
	auditor := audit.New(af.Name())
	t.Cleanup(func() { auditor.Close() })

	srv, err := NewServer(sock, session.NewStore(), scheduler.New(8192),
		mcp.NewHost(nil, auditor), auditor, fakeRecapProvider{}, false, 0, "", "")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { srv.Close(); os.Remove(sock) })
	return srv, NewClient(sock)
}

func TestSessionRecap(t *testing.T) {
	t.Run("idle session with a conversation gets a recap", func(t *testing.T) {
		srv, client := newRecapServer(t, "/tmp/inari-test-recap-idle.sock")
		defer client.Close()

		sess := session.New("test")
		sess.Model = "gemma4:e2b"
		sess.AppendMessage(provider.Message{Role: "user", Content: "help me with X"})
		sess.AppendMessage(provider.Message{Role: "assistant", Content: "sure, do Y"})
		sess.UpdatedAt = time.Now().Add(-11 * time.Minute) // gone idle past the threshold
		srv.store.Add(sess)

		got, err := client.Recap(sess.ID)
		if err != nil {
			t.Fatalf("Recap: %v", err)
		}
		if got != "you were debugging the recap RPC." {
			t.Fatalf("recap = %q, want the generated summary", got)
		}
	})

	t.Run("fresh (active) session returns empty", func(t *testing.T) {
		srv, client := newRecapServer(t, "/tmp/inari-test-recap-fresh.sock")
		defer client.Close()

		sess := session.New("test")
		sess.Model = "gemma4:e2b"
		sess.AppendMessage(provider.Message{Role: "user", Content: "hi"})
		sess.AppendMessage(provider.Message{Role: "assistant", Content: "hello"})
		srv.store.Add(sess) // UpdatedAt is now, so not idle

		got, err := client.Recap(sess.ID)
		if err != nil {
			t.Fatalf("Recap: %v", err)
		}
		if got != "" {
			t.Fatalf("fresh session recap = %q, want empty", got)
		}
	})

	t.Run("idle session with no conversation returns empty", func(t *testing.T) {
		srv, client := newRecapServer(t, "/tmp/inari-test-recap-empty.sock")
		defer client.Close()

		sess := session.New("test")
		sess.Model = "gemma4:e2b"
		sess.UpdatedAt = time.Now().Add(-11 * time.Minute) // idle, but only the system message
		srv.store.Add(sess)

		got, err := client.Recap(sess.ID)
		if err != nil {
			t.Fatalf("Recap: %v", err)
		}
		if got != "" {
			t.Fatalf("no-conversation recap = %q, want empty", got)
		}
	})
}
