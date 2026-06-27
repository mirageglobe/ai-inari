// Package audit writes an append-only JSONL log of every tool-call the daemon handles.
// each entry records the RPC method, params, and a timestamp for operator inspection.
//
// it owns:
//   - the audit Entry format and the append-only writer.
//
// it does NOT own:
//   - the tool-calls it records (internal/mcp) or their execution.
//   - log rotation or retention policy (left to the operator / host).
package audit
