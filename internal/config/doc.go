// Package config loads and holds the daemon configuration from config.json.
// it defines the socket path, memory budget, Ollama URL, MCP connectors, model
// assignments, and theme.
//
// it owns:
//   - the on-disk config schema, loading, defaulting, and saving.
//
// it does NOT own:
//   - the behaviour of the values it carries (sockets, budgets, connectors live in
//     internal/ipc, internal/scheduler, internal/mcp respectively).
//   - runtime session state (internal/session).
package config
