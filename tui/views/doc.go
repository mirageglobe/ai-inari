// Package views contains the individual screen views rendered by the inari TUI:
// Agents (session table), Models (model selector), Logs (token stream), Describe
// (session metadata), and Chat (head-inari conversation), plus the shared top bar,
// footer, hint bar, help overlay, and theme palette.
//
// it owns:
//   - each view's Init/Update/View, its local state, and view-specific input handling.
//   - shared rendering helpers (header, footer, hints, help) and the theme styles.
//
// it does NOT own:
//   - view routing between screens (tui).
//   - IPC transport (internal/ipc) or any daemon-side state (internal/*); views only
//     hold the data handed to them and the messages they emit.
package views
