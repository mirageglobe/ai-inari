// Package config loads and holds the daemon configuration from config.json.
// it defines the socket path, memory budget, Ollama URL, MCP connectors, model
// assignments, and theme.
//
// it owns:
//   - the on-disk config schema, loading, defaulting, and saving.
//   - the restricted per-project overlay schema (.inari/config.json) and its
//     loader; it defines which fields a project directory may override and
//     deliberately excludes all infra/security fields.
//
// it does NOT own:
//   - the behaviour of the values it carries (sockets, budgets, connectors live in
//     internal/ipc, internal/scheduler, internal/mcp respectively).
//   - runtime session state (internal/session).
package config
