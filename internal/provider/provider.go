// Package provider defines the inference backend abstraction for inarid.
// it owns the Provider interface and the shared message/model types that cross
// the boundary between the daemon core and any concrete backend.
//
// what it owns:
//   - Provider interface (Chat, ChatStream, LoadModel, UnloadModel, ListModels, ListRunning, Ping)
//   - shared wire types: Message, Tool, ChatRequest, ChatResponse, Model, RunningModel
//
// what it does NOT own:
//   - backend-specific HTTP / protocol logic (lives in internal/ollama, future packages)
//   - session state or persistence (internal/session)
//   - IPC dispatch (internal/ipc)
package provider

// Message is a single chat turn.
// ToolCalls is populated on assistant messages when the model requests a function call.
type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall is a single function invocation requested by the model.
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction carries the name and arguments for a tool call.
type ToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// Tool is declared in a ChatRequest to advertise a callable function to the model.
type Tool struct {
	Type     string       `json:"type"` // always "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction is the function definition inside a Tool.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
}

// ToolParameters describes the JSON schema for a tool's input.
type ToolParameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

// Property is a single field in a tool's parameter schema.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ChatRequest is the input to a chat call.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Tools    []Tool    `json:"tools,omitempty"`
}

// ChatResponse is a single streamed or complete response chunk.
type ChatResponse struct {
	Message Message `json:"message"`
	Done    bool    `json:"done"`
}

// Model is a locally available model reported by the backend.
type Model struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// RunningModel is a model currently loaded in backend memory.
type RunningModel struct {
	Name      string `json:"name"`
	SizeVRAM  int64  `json:"size_vram"`
	ExpiresAt string `json:"expires_at"`
}

// Provider is the interface inarid's core uses to talk to any inference backend.
// the concrete implementation is selected at startup via config.json "provider" field.
type Provider interface {
	// Ping checks that the backend is reachable.
	Ping() error
	// Chat sends a blocking single-turn request and returns the full reply.
	Chat(model string, messages []Message) (string, error)
	// ChatStream sends a request and yields response chunks via out until done.
	ChatStream(req ChatRequest, out chan<- ChatResponse) error
	// LoadModel warms the model into backend memory.
	LoadModel(model string) error
	// UnloadModel evicts the model from backend memory.
	UnloadModel(model string) error
	// ListModels returns all models available to the backend.
	ListModels() ([]Model, error)
	// ListRunning returns models currently loaded in backend memory.
	ListRunning() ([]RunningModel, error)
}
