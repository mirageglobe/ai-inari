package ipc

import (
	"encoding/json"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/session"
)

// TestSessionAssignNilProviderNoPanic asserts session.assign returns the clean
// "provider not configured" error (-32603) instead of panicking on ListModels
// when the daemon has no provider. guards the missing providerErr check.
func TestSessionAssignNilProviderNoPanic(t *testing.T) {
	srv := &Server{store: session.NewStore()} // provider left nil
	sess := session.New("t")
	srv.store.Add(sess)

	req := Request{
		JSONRPC: "2.0",
		Method:  "session.assign",
		ID:      1,
		Params:  json.RawMessage(`{"id":"` + sess.ID + `","model":"whatever"}`),
	}
	resp := srv.dispatch(req)
	if resp.Error == nil {
		t.Fatal("expected an error assigning with a nil provider, got none")
	}
	if resp.Error.Code != -32603 {
		t.Fatalf("expected provider-not-configured code -32603, got %d (%s)", resp.Error.Code, resp.Error.Message)
	}
}
