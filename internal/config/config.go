// Package config loads and holds the daemon configuration from config.json.
// It defines the socket path, memory budget, Ollama URL, MCP connectors, and model assignments.
package config

import (
	"encoding/json"
	"os"
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
	MCPConnectors  []MCPConnector `json:"mcp_connectors"`
	Models         Models         `json:"models"`
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
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
