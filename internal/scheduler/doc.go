// Package scheduler enforces the memory budget across concurrent Ollama sessions.
// it uses a semaphore to gate how many sessions run in parallel based on configured
// MB limits per tier.
//
// it owns:
//   - the per-tier memory cost table and the budget semaphore.
//   - admission control for starting a new session under the budget.
//
// it does NOT own:
//   - session state or history (internal/session).
//   - actually loading/unloading models (internal/provider, internal/ollama).
//   - the budget value itself (internal/config).
package scheduler
