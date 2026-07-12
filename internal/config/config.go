package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type MCPConnector struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Models struct {
	Thinker string `json:"thinker"`
	Worker  string `json:"worker"`
	Sensor  string `json:"sensor"`
}

type Config struct {
	Socket         string         `json:"socket"`
	MemoryBudgetMB int            `json:"memory_budget_mb"`
	OllamaBaseURL  string         `json:"ollama_base_url"`
	DataDir        string         `json:"data_dir"`
	MCPConnectors  []MCPConnector `json:"mcp_connectors"`
	Models         Models         `json:"models"`
	Theme          string         `json:"theme,omitempty"`
	// idle_shutdown_mins: minutes of no client activity after which the daemon
	// exits on its own. 0 falls back to the 30 min default; a negative value
	// disables auto-shutdown entirely.
	IdleShutdownMins int `json:"idle_shutdown_mins"`
}

var defaults = &Config{
	Socket:         "/tmp/inari.sock",
	MemoryBudgetMB: 8192,
	OllamaBaseURL:  "http://localhost:11434",
	MCPConnectors:  []MCPConnector{},
	Models: Models{
		Thinker: "gemma4:e2b",
		Worker:  "bonsai:4b",
		Sensor:  "qwen3-nano",
	},
	Theme:            "slate",
	IdleShutdownMins: 30,
}

// Load reads config from path. if the file does not exist it is created with
// defaults so the user has a starting point to edit.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		if mkErr := os.MkdirAll(filepath.Dir(path), 0755); mkErr != nil {
			return nil, mkErr
		}
		cfg := *defaults
		if saveErr := cfg.Save(path); saveErr != nil {
			return nil, saveErr
		}
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes the config back to path with indented JSON.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
