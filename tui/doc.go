// Package tui is the root Bubble Tea model for fox (the terminal user interface).
//
// it owns:
//   - view routing across herd, models, logs, describe, and chat.
//   - top-level message dispatch and delegation of input/rendering to each view.
//   - the IPC client wiring used by the views.
//
// it does NOT own:
//   - the rendering of individual views (tui/views).
//   - daemon state, sessions, or inference (internal/*).
package tui
