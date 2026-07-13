package ipc

import (
	"encoding/json"
	"fmt"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// ListModels returns models available in Ollama via inarid.
func (c *Client) ListModels() ([]provider.Model, error) {
	resp, err := c.Call("ollama.models", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("%s", resp.Error.Message)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}
	var models []provider.Model
	if err := json.Unmarshal(b, &models); err != nil {
		return nil, err
	}
	return models, nil
}

// ListRunning returns models currently loaded in Ollama memory.
func (c *Client) ListRunning() ([]provider.RunningModel, error) {
	resp, err := c.Call("ollama.running", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("%s", resp.Error.Message)
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}
	var models []provider.RunningModel
	if err := json.Unmarshal(b, &models); err != nil {
		return nil, err
	}
	return models, nil
}

// LoadModel warms up the named model in Ollama memory via inarid.
func (c *Client) LoadModel(model string) error {
	resp, err := c.Call("ollama.load", map[string]string{"model": model})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.Error.Message)
	}
	return nil
}

// UnloadModel evicts the named model from Ollama memory via inarid.
func (c *Client) UnloadModel(model string) error {
	resp, err := c.Call("ollama.unload", map[string]string{"model": model})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.Error.Message)
	}
	return nil
}

// DeleteModel removes the named model from Ollama's local disk storage via
// inarid. distinct from UnloadModel (memory eviction); this frees disk and
// requires a re-pull before the model can be used again.
func (c *Client) DeleteModel(model string) error {
	resp, err := c.Call("ollama.delete", map[string]string{"model": model})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s", resp.Error.Message)
	}
	return nil
}

// ModelCaps returns the capability tags for a model (e.g. "tools", "vision").
// returns an empty slice when the model is unknown or the backend does not support introspection.
func (c *Client) ModelCaps(model string) ([]string, error) {
	resp, err := c.Call("ollama.show", map[string]string{"model": model})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return []string{}, nil
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return []string{}, nil
	}
	var caps []string
	if err := json.Unmarshal(b, &caps); err != nil {
		return []string{}, nil
	}
	return caps, nil
}
