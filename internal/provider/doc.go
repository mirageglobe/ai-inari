// Package provider defines the inference backend abstraction for inarid.
// it owns the Provider interface and the shared message/model types that cross
// the boundary between the daemon core and any concrete backend.
//
// it owns:
//   - the Provider interface (Chat, ChatStream, LoadModel, UnloadModel, ListModels,
//     ListRunning, Ping, ModelCaps, PullModel, DeleteModel).
//   - shared wire types: Message, Tool, ChatRequest, ChatResponse, Model, RunningModel.
//
// it does NOT own:
//   - backend-specific HTTP / protocol logic (internal/ollama, future packages).
//   - session state or persistence (internal/session).
//   - IPC dispatch (internal/ipc).
package provider
