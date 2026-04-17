package session

import (
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	store := NewStore()

	sess := &Session{
		ID:        "abc123",
		Model:     "bonsai:4b",
		Tier:      TierWorker,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	store.Add(sess)

	got, ok := store.Get("abc123")
	if !ok {
		t.Fatal("Get: session not found")
	}
	if got.Model != "bonsai:4b" {
		t.Errorf("model = %q, want bonsai:4b", got.Model)
	}

	list := store.List()
	if len(list) != 1 {
		t.Errorf("list len = %d, want 1", len(list))
	}

	store.Remove("abc123")
	if _, ok := store.Get("abc123"); ok {
		t.Error("session still present after Remove")
	}
}
