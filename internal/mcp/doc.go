// Package mcp manages Model Context Protocol connector child processes.
// each connector (filesystem, search, SQL) is spawned via stdio and its tool-calls
// are routed through the audit log.
//
// it owns:
//   - connector process lifecycle (spawn, stdio wiring, teardown).
//   - dispatch of tool-calls to the correct connector.
//
// it does NOT own:
//   - the audit record format or storage (internal/audit).
//   - connector configuration shape (internal/config).
//   - session state (internal/session) or model dispatch (internal/provider).
package mcp
