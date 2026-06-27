// Package ipc implements the JSON-RPC 2.0 transport between fox and inarid over a
// Unix Domain Socket. Client is used by fox; Server is used by inarid. both live here
// to keep the wire protocol defined in one place.
//
// it owns:
//   - the JSON-RPC framing, request/response dispatch, and UDS connection handling.
//   - the wire-level type shapes exchanged over the socket (SessionInfo, etc.).
//
// it does NOT own:
//   - business logic for sessions (internal/session) or models (internal/provider).
//   - inference backend behaviour (internal/ollama).
//   - the inference type definitions it transports (internal/provider).
package ipc
