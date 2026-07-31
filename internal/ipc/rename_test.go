package ipc

import (
	"testing"

	"github.com/mirageglobe/inari/internal/session"
)

// TestSessionRename covers the happy path (name updated in place, other fields
// preserved), rejection of an empty name, and an unknown session id.
func TestSessionRename(t *testing.T) {
	t.Run("valid name updates in place and preserves other fields", func(t *testing.T) {
		srv, client := newSetCwdServer(t, "/tmp/inari-test-rename-ok.sock")
		defer client.Close()

		sess := session.New("old")
		sess.Model = "gemma4:e2b"
		srv.store.Add(sess)

		info, err := client.Rename(sess.ID, "new")
		if err != nil {
			t.Fatalf("Rename: %v", err)
		}
		if info.Name != "new" {
			t.Fatalf("returned name = %q, want %q", info.Name, "new")
		}
		got, _ := srv.store.Get(sess.ID)
		if got.Name != "new" {
			t.Fatalf("stored name = %q, want %q", got.Name, "new")
		}
		if got.Model != "gemma4:e2b" {
			t.Fatalf("rename should preserve model, got %q", got.Model)
		}
	})

	t.Run("rejects an empty name", func(t *testing.T) {
		srv, client := newSetCwdServer(t, "/tmp/inari-test-rename-empty.sock")
		defer client.Close()

		sess := session.New("old")
		srv.store.Add(sess)

		if _, err := client.Rename(sess.ID, ""); err == nil {
			t.Fatal("expected an error for an empty name, got nil")
		}
		got, _ := srv.store.Get(sess.ID)
		if got.Name != "old" {
			t.Fatalf("name should be unchanged on error, got %q", got.Name)
		}
	})

	t.Run("rejects an unknown session id", func(t *testing.T) {
		_, client := newSetCwdServer(t, "/tmp/inari-test-rename-nosess.sock")
		defer client.Close()

		if _, err := client.Rename("deadbeef", "x"); err == nil {
			t.Fatal("expected an error for an unknown session, got nil")
		}
	})
}
