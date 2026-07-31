package ollama

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mirageglobe/inari/internal/provider"
)

// TestKeepAliveInChatBody asserts SetKeepAlive causes keep_alive to be sent on
// chat requests, and that it is omitted entirely when unset.
func TestKeepAliveInChatBody(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = nil
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"hi"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetKeepAlive("5m")
	if _, err := c.Chat("m", []provider.Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if gotBody["keep_alive"] != "5m" {
		t.Fatalf("keep_alive = %v, want 5m", gotBody["keep_alive"])
	}

	// unset -> field omitted from the request body.
	c2 := NewClient(srv.URL)
	if _, err := c2.Chat("m", []provider.Message{{Role: "user", Content: "x"}}); err != nil {
		t.Fatalf("Chat (no keep_alive): %v", err)
	}
	if _, present := gotBody["keep_alive"]; present {
		t.Fatal("keep_alive should be omitted when unset")
	}
}
