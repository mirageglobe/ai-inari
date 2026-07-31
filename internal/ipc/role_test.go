package ipc

import (
	"testing"

	"github.com/mirageglobe/inari/internal/session"
)

// TestSessionSetRole covers a valid role, rejection of an unknown role (leaving
// the prior role intact), clearing with "", and an unknown session.
func TestSessionSetRole(t *testing.T) {
	srv, client := newSetCwdServer(t, "/tmp/inari-test-setrole.sock")
	defer client.Close()

	sess := session.New("s")
	srv.store.Add(sess)

	info, err := client.SetRole(sess.ID, "coding")
	if err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if info.Role != "coding" {
		t.Fatalf("role = %q, want coding", info.Role)
	}

	if _, err := client.SetRole(sess.ID, "wizard"); err == nil {
		t.Fatal("expected an error for an invalid role, got nil")
	}
	if got, _ := srv.store.Get(sess.ID); got.Role != "coding" {
		t.Fatalf("role should stay coding after a rejected set, got %q", got.Role)
	}

	if info, _ = client.SetRole(sess.ID, ""); info.Role != "" {
		t.Fatalf("empty role should clear, got %q", info.Role)
	}

	if _, err := client.SetRole("deadbeef", "coding"); err == nil {
		t.Fatal("expected an error for an unknown session, got nil")
	}
}
