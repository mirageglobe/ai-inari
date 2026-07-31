package ipc

import (
	"strings"
	"testing"

	"github.com/mirageglobe/inari/internal/provider"
	"github.com/mirageglobe/inari/internal/session"
)

// staleMarker returns the injected cwd-change marker from a session's history, or
// "" if none was appended. index 0 is the system prompt, so the marker (also role
// "system") is looked for after it.
func staleMarker(sess *session.Session) string {
	h := sess.ChatHistory()
	for i := 1; i < len(h); i++ {
		if h[i].Role == "system" && strings.Contains(h[i].Content, "stale") {
			return h[i].Content
		}
	}
	return ""
}

// TestSetCwdInjectsStaleMarker asserts that changing the cwd of a session that
// already has conversation appends a stale-history marker naming the new cwd, so
// the model re-runs tools instead of regurgitating the old directory's listing.
func TestSetCwdInjectsStaleMarker(t *testing.T) {
	srv, client := newSetCwdServer(t, "/tmp/inari-test-setcwd-marker.sock")
	defer client.Close()

	sess := session.New("test")
	sess.AppendMessage(provider.Message{Role: "user", Content: "list files"})
	sess.AppendMessage(provider.Message{Role: "assistant", Content: "a.go\nb.go"})
	srv.store.Add(sess)

	dir := t.TempDir()
	if _, err := client.SetCwd(sess.ID, dir); err != nil {
		t.Fatalf("SetCwd: %v", err)
	}
	got, _ := srv.store.Get(sess.ID)
	marker := staleMarker(got)
	if marker == "" {
		t.Fatal("expected a stale-history marker after cwd change, found none")
	}
	if !strings.Contains(marker, dir) {
		t.Fatalf("marker should name the new cwd %q, got %q", dir, marker)
	}
}

// TestSetCwdNoMarkerWithoutHistory asserts no marker is injected when the session
// has only its system prompt (nothing stale to warn about).
func TestSetCwdNoMarkerWithoutHistory(t *testing.T) {
	srv, client := newSetCwdServer(t, "/tmp/inari-test-setcwd-nomarker.sock")
	defer client.Close()

	sess := session.New("test") // system prompt only, no conversation
	srv.store.Add(sess)

	dir := t.TempDir()
	if _, err := client.SetCwd(sess.ID, dir); err != nil {
		t.Fatalf("SetCwd: %v", err)
	}
	got, _ := srv.store.Get(sess.ID)
	if m := staleMarker(got); m != "" {
		t.Fatalf("expected no marker without prior history, got %q", m)
	}
}
