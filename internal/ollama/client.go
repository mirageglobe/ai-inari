// Package ollama is the HTTP client for the local Ollama inference server.
// It covers health checks, model listing, and streaming chat requests to /api/chat.
package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

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

// ToolFunction is the function definition inside a Tool.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
}

// Tool is declared in a ChatRequest to advertise a callable function to the model.
type Tool struct {
	Type     string       `json:"type"` // always "function"
	Function ToolFunction `json:"function"`
}

// ToolCallFunction carries the name and arguments returned by the model for a tool call.
type ToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolCall is a single function invocation requested by the model.
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// Message is a single chat message.
// ToolCalls is populated on assistant messages when the model requests a function call.
type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ChatRequest maps to the Ollama /api/chat payload.
// Tools declares the functions the model may call; omit for sessions without tool support.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Tools    []Tool    `json:"tools,omitempty"`
}

// ChatResponse is a single streamed chunk from Ollama.
type ChatResponse struct {
	Message Message `json:"message"`
	Done    bool    `json:"done"`
}

// Model is a locally available Ollama model.
type Model struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// RunningModel is a model currently loaded in Ollama memory (/api/ps).
type RunningModel struct {
	Name      string `json:"name"`
	SizeVRAM  int64  `json:"size_vram"`
	ExpiresAt string `json:"expires_at"`
}

// Client talks to the Ollama HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{}}
}

// Ping returns nil if Ollama is reachable.
func (c *Client) Ping() error {
	resp, err := c.http.Get(c.baseURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("ollama unreachable at %s: %w", c.baseURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// getModels fetches endpoint and decodes the top-level "models" array into out.
// Both /api/tags and /api/ps wrap their lists in {"models": [...]}, so one helper serves both.
func (c *Client) getModels(endpoint string, out any) error {
	resp, err := c.http.Get(c.baseURL + endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var wrapper struct {
		Models json.RawMessage `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return err
	}
	return json.Unmarshal(wrapper.Models, out)
}

// ListModels returns all locally available models.
func (c *Client) ListModels() ([]Model, error) {
	var models []Model
	return models, c.getModels("/api/tags", &models)
}

// ListRunning returns models currently loaded in Ollama memory.
func (c *Client) ListRunning() ([]RunningModel, error) {
	var models []RunningModel
	return models, c.getModels("/api/ps", &models)
}

// LoadModel warms up the model in Ollama memory.
// Ollama requires a prompt field; an empty prompt with stream=false loads the model without generating output.
func (c *Client) LoadModel(model string) error {
	body, _ := json.Marshal(map[string]any{"model": model, "prompt": "", "stream": false})
	resp, err := c.http.Post(c.baseURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: status %d", resp.StatusCode)
	}
	return nil
}

// UnloadModel evicts the model from Ollama memory. keep_alive=0 is Ollama's documented
// mechanism for immediate eviction; there is no dedicated unload endpoint.
func (c *Client) UnloadModel(model string) error {
	body, _ := json.Marshal(map[string]any{"model": model, "keep_alive": 0})
	resp, err := c.http.Post(c.baseURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: status %d", resp.StatusCode)
	}
	return nil
}

// Chat sends a single blocking request and returns the full reply.
func (c *Client) Chat(model string, messages []Message) (string, error) {
	req := ChatRequest{Model: model, Messages: messages, Stream: false}
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Post(c.baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama: status %d", resp.StatusCode)
	}
	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Message.Content, nil
}

// ChatStream sends a chat request and yields response chunks via a channel.
func (c *Client) ChatStream(req ChatRequest, out chan<- ChatResponse) error {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := c.http.Post(c.baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk ChatResponse
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		out <- chunk
		if chunk.Done {
			break
		}
	}
	return scanner.Err()
}
