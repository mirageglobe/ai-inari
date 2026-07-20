// Package main is the inari unified entry point.
//
// it owns:
//   - subcommand parsing and dispatch: start, daemon, tui, stop, status, version.
//   - start:   fork the daemon in the background then launch the TUI.
//   - daemon:  run the IPC server; detaches into the background by default,
//     -f/--foreground runs attached, --child is the internal forked-worker marker.
//   - tui:     run the terminal UI only (assumes the daemon is already running).
//   - stop:    send SIGTERM to the running daemon.
//   - status:  report whether the daemon is running.
//   - process lifecycle glue (PID file, forking) at the binary boundary.
//
// it does NOT own:
//   - IPC transport or RPC handlers (internal/ipc).
//   - session state or persistence (internal/session).
//   - inference backends (internal/ollama, internal/provider).
//   - UI rendering or input handling (tui, tui/views).
package main
