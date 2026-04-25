// Package ollama is the Ollama HTTP backend implementation of provider.Provider.
// it translates the provider interface into HTTP calls against the Ollama REST API
// (/api/chat, /api/tags, /api/ps, /api/generate).
//
// what it owns: HTTP transport, Ollama-specific request/response encoding.
// what it does NOT own: type definitions for messages or models (internal/provider),
// session state (internal/session), or IPC dispatch (internal/ipc).
package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// compile-time check: *Client must implement provider.Provider.
var _ provider.Provider = (*Client)(nil)

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
func (c *Client) ListModels() ([]provider.Model, error) {
	var models []provider.Model
	return models, c.getModels("/api/tags", &models)
}

// ListRunning returns models currently loaded in Ollama memory.
func (c *Client) ListRunning() ([]provider.RunningModel, error) {
	var models []provider.RunningModel
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
func (c *Client) Chat(model string, messages []provider.Message) (string, error) {
	req := provider.ChatRequest{Model: model, Messages: messages, Stream: false}
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
	var result provider.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Message.Content, nil
}

// ChatStream sends a chat request and yields response chunks via a channel.
func (c *Client) ChatStream(req provider.ChatRequest, out chan<- provider.ChatResponse) error {
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
		var chunk provider.ChatResponse
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
