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

// Message is a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest maps to the Ollama /api/chat payload.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
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

// UnloadModel evicts the model from Ollama memory by sending keep_alive=0.
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
