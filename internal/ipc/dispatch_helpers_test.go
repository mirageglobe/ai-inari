package ipc

import (
	"encoding/json"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/session"
)

// TestDecodeParamsReturnsInvalidParams asserts malformed params now return the
// spec-correct -32602 (invalid params), replacing the old -32600/-32602 split.
func TestDecodeParamsReturnsInvalidParams(t *testing.T) {
	srv := &Server{store: session.NewStore()}
	req := Request{JSONRPC: "2.0", Method: "session.delete", ID: 1, Params: json.RawMessage(`not-json`)}
	resp := srv.dispatch(req)
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("malformed params should return -32602, got %+v", resp.Error)
	}
}

// TestGetSessionMissReturnsNotFound asserts the shared getSession helper returns a
// -32602 "session not found" for an unknown id.
func TestGetSessionMissReturnsNotFound(t *testing.T) {
	srv := &Server{store: session.NewStore()}
	req := Request{JSONRPC: "2.0", Method: "session.unassign", ID: 1, Params: json.RawMessage(`{"id":"nope"}`)}
	resp := srv.dispatch(req)
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("unknown session should return -32602, got %+v", resp.Error)
	}
	if resp.Error.Message != "session not found" {
		t.Fatalf(`expected "session not found", got %q`, resp.Error.Message)
	}
}
