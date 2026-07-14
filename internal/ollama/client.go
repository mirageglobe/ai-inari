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
	"strings"

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

// NewClientWithAuth builds a client whose every request carries an optional
// Authorization bearer token and any extra static headers, so inarid can talk to
// an authenticated or non-default backend (LM Studio, llama.cpp, a cloud proxy)
// selected via a config endpoint profile. an empty apiKey and nil headers behave
// exactly like NewClient.
func NewClientWithAuth(baseURL, apiKey string, headers map[string]string) *Client {
	if apiKey == "" && len(headers) == 0 {
		return NewClient(baseURL)
	}
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Transport: &authTransport{apiKey: apiKey, headers: headers, base: http.DefaultTransport}},
	}
}

// authTransport injects auth/static headers on every outbound request, so the
// header logic lives in one place rather than at each Get/Post call site.
type authTransport struct {
	apiKey  string
	headers map[string]string
	base    http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.base.RoundTrip(req)
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

// DeleteModel removes the model from Ollama's local disk storage via
// DELETE /api/delete. distinct from UnloadModel (memory eviction); this frees
// disk and requires a re-pull before the model can be used again.
func (c *Client) DeleteModel(model string) error {
	body, _ := json.Marshal(map[string]string{"model": model})
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
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

// ModelContextLength calls /api/show and returns the model's maximum context
// window in tokens, read from the architecture-prefixed "<arch>.context_length"
// key in model_info. returns 0 (not an error) when the model is unknown or the
// field is absent, so callers can treat it as "use the backend default".
func (c *Client) ModelContextLength(model string) (int, error) {
	body, _ := json.Marshal(map[string]string{"name": model})
	resp, err := c.http.Post(c.baseURL+"/api/show", "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, nil
	}
	var result struct {
		ModelInfo map[string]json.RawMessage `json:"model_info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, nil
	}
	// the key is "<architecture>.context_length" (e.g. "llama.context_length");
	// scan by suffix so we don't need to resolve the architecture first.
	for k, v := range result.ModelInfo {
		if strings.HasSuffix(k, ".context_length") {
			var n int
			if json.Unmarshal(v, &n) == nil {
				return n, nil
			}
		}
	}
	return 0, nil
}
