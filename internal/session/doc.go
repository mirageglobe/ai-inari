// Package session tracks the lifecycle of every model session owned by inarid.
// sessions survive inari detaching and can be reconnected by ID.
//
// it owns:
//   - session creation, lookup, and removal.
//   - per-session conversation history and its persistence to disk.
//   - session metadata (name, model, cwd, token accounting).
//
// it does NOT own:
//   - the inference backend that produces responses (internal/provider, internal/ollama).
//   - IPC transport (internal/ipc) or memory budgeting (internal/scheduler).
//   - tool-call execution or auditing (internal/mcp, internal/audit).
package session
