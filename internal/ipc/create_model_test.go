package ipc

import (
	"encoding/json"
	"testing"

	"github.com/mirageglobe/inari/internal/session"
)

// TestSessionCreateUsesConfiguredModel asserts a new session takes the daemon's
// configured thinker-tier default (s.defaultModel), not the hardcoded fallback, so a
// config models.thinker is honored for new sessions.
func TestSessionCreateUsesConfiguredModel(t *testing.T) {
	srv := &Server{store: session.NewStore(), defaultModel: "custom:model"}
	req := Request{JSONRPC: "2.0", Method: "session.create", ID: 1, Params: json.RawMessage(`{"name":"t"}`)}
	resp := srv.dispatch(req)
	if resp.Error != nil {
		t.Fatalf("create failed: %v", resp.Error)
	}
	info, ok := resp.Result.(SessionInfo)
	if !ok {
		t.Fatalf("unexpected result type %T", resp.Result)
	}
	if info.Model != "custom:model" {
		t.Fatalf("new session should use the configured default model, got %q", info.Model)
	}
}
