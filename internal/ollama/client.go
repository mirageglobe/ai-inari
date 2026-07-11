// client.go owns the Ollama REST client core: transport, error decoding, health
// check, model listing, and model load/unload/caps. it does NOT own the chat and
// pull calls (chat.go).

package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// compile-time check: *Client must implement provider.Provider.
var _ provider.Provider = (*Client)(nil)

// Client talks to the Ollama HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
	verbose bool
}

func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{}}
}

func (c *Client) SetVerbose(v bool) { c.verbose = v }

// ollamaError reads the response body and returns a descriptive error that
// includes the ollama message when available (e.g. "model not found").
func ollamaError(resp *http.Response) error {
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &payload) == nil && payload.Error != "" {
		return fmt.Errorf("ollama: status %d: %s", resp.StatusCode, payload.Error)
	}
	return fmt.Errorf("ollama: status %d", resp.StatusCode)
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
	if resp.StatusCode != http.StatusOK {
		return ollamaError(resp)
	}
	resp.Body.Close()
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
	if resp.StatusCode != http.StatusOK {
		return ollamaError(resp)
	}
	resp.Body.Close()
	return nil
}

// ModelCaps calls /api/show and returns the model's declared capability tags
// (e.g. "tools", "vision"). returns an empty slice when the field is absent or
// when the model is not found rather than an error, so callers can ignore unknowns.
func (c *Client) ModelCaps(model string) ([]string, error) {
	body, _ := json.Marshal(map[string]string{"name": model})
	resp, err := c.http.Post(c.baseURL+"/api/show", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return []string{}, nil
	}
	var result struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return []string{}, nil
	}
	return result.Capabilities, nil
}
