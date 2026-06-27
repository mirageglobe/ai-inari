// Package ollama is the Ollama HTTP backend implementation of provider.Provider.
// it translates the provider interface into HTTP calls against the Ollama REST API
// (/api/chat, /api/tags, /api/ps, /api/generate, /api/show).
//
// it owns:
//   - HTTP transport and Ollama-specific request/response encoding.
//   - streaming decode of the chat endpoint.
//
// it does NOT own:
//   - the message/model type definitions it encodes (internal/provider).
//   - session state (internal/session) or IPC dispatch (internal/ipc).
package ollama
