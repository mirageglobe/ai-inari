// dispatch_helpers.go owns the small shared prologue helpers every session/chat
// handler used to hand-roll: JSON-RPC invalid-params responses, param decoding,
// and session lookup. centralising them removes ~6 lines of boilerplate per
// handler and gives one consistent error code (-32602 for bad params) instead of
// the -32600/-32602 split handlers had grown. it does NOT own the switch
// (dispatch.go) or any handler logic (dispatch_session.go / dispatch_chat.go).

package ipc

import "encoding/json"

import "github.com/mirageglobe/inari/internal/session"

// badParams builds a JSON-RPC invalid-params (-32602) error response. per the spec,
// -32602 is the code for malformed/invalid params (previously some handlers used
// -32600, which is Invalid Request, i.e. a malformed envelope, not bad params).
func badParams(req Request, msg string) Response {
	return Response{JSONRPC: "2.0", Error: &Error{Code: -32602, Message: msg}, ID: req.ID}
}

// decodeParams unmarshals req.Params into dst. on failure it returns a non-nil
// invalid-params response the caller must return immediately; nil means success.
func decodeParams(req Request, dst any) *Response {
	if err := json.Unmarshal(req.Params, dst); err != nil {
		r := badParams(req, "invalid params")
		return &r
	}
	return nil
}

// getSession looks a session up by id. on miss it returns a non-nil
// session-not-found response the caller must return immediately; nil means found.
func (s *Server) getSession(req Request, id string) (*session.Session, *Response) {
	sess, ok := s.store.Get(id)
	if !ok {
		r := badParams(req, "session not found")
		return nil, &r
	}
	return sess, nil
}
