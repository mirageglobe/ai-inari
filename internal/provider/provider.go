package provider

import "context"

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
// Options carries backend runtime options (e.g. num_ctx); omitted when empty so
// the backend falls back to its own defaults.
type ChatRequest struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Tools    []Tool         `json:"tools,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
}

// ChatResponse is a single streamed or complete response chunk.
type ChatResponse struct {
	Message Message `json:"message"`
	Done    bool    `json:"done"`

	// inference counters, populated only on the final chunk; durations are in
	// nanoseconds. the backend returns these for free on every turn and they are
	// the only cost signal available for tier selection, so they are decoded here
	// rather than discarded. see Metrics for the derived view.
	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
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

// PullProgress is one status update from an in-progress model download.
// Completed/Total are byte counts, populated only during the "downloading" status.
type PullProgress struct {
	Status    string `json:"status"`
	Completed int64  `json:"completed,omitempty"`
	Total     int64  `json:"total,omitempty"`
}

// Provider is the interface inarid's core uses to talk to any inference backend.
// the concrete implementation is constructed at startup in cmd/inari (currently
// always the Ollama client); config.json's "provider" field selects an endpoint
// URL profile, not the implementation. a real impl-selection factory is tracked in
// docs/architecture-review.md (S6).
type Provider interface {
	// Ping checks that the backend is reachable.
	Ping() error
	// Chat sends a blocking single-turn request and returns the full reply.
	Chat(model string, messages []Message) (string, error)
	// ChatStream sends a request and yields response chunks via out until done.
	// cancelling ctx aborts the in-flight request so the caller can interrupt a
	// long generation; the implementation returns ctx.Err() in that case.
	ChatStream(ctx context.Context, req ChatRequest, out chan<- ChatResponse) error
	// LoadModel warms the model into backend memory.
	LoadModel(model string) error
	// UnloadModel evicts the model from backend memory.
	UnloadModel(model string) error
	// ListModels returns all models available to the backend.
	ListModels() ([]Model, error)
	// ListRunning returns models currently loaded in backend memory.
	ListRunning() ([]RunningModel, error)
	// ModelCaps returns the capability tags for a model (e.g. "tools", "vision").
	// returns an empty slice when the backend does not support capability introspection.
	ModelCaps(model string) ([]string, error)
	// PullModel downloads model, streaming progress updates via out until the
	// final "success" status; out is not closed by the implementation.
	PullModel(model string, out chan<- PullProgress) error
	// DeleteModel removes model from the backend's local disk storage.
	// distinct from UnloadModel (memory eviction only); irreversible without a re-pull.
	DeleteModel(model string) error
	// ModelContextLength returns the model's maximum context window in tokens,
	// as declared by the backend. returns 0 (not an error) when unknown.
	ModelContextLength(model string) (int, error)
}
