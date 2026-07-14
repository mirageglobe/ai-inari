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

// Endpoint is a named inference-backend profile: the base URL of an OpenAI/Ollama
// compatible server plus optional auth. Headers are sent on every request, so an
// api_key can also be supplied there if a backend expects a non-standard header.
type Endpoint struct {
	BaseURL string            `json:"base_url"`
	APIKey  string            `json:"api_key,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Ollama holds runtime tuning for the Ollama backend. KeepAlive is applied by
// inarid on every chat request (how long an idle model stays loaded, e.g. "5m";
// empty uses Ollama's own default). MaxLoadedModels and NumParallel are
// server-start settings inarid cannot set on an external `ollama serve`; they are
// recorded here so `inari doctor` can surface them as the host env vars
// OLLAMA_MAX_LOADED_MODELS / OLLAMA_NUM_PARALLEL to set for the desired
// memory/throughput trade-off.
type Ollama struct {
	KeepAlive       string `json:"keep_alive,omitempty"`
	MaxLoadedModels int    `json:"max_loaded_models,omitempty"`
	NumParallel     int    `json:"num_parallel,omitempty"`
}

// Shell configures the execute_shell_command tool gate.
type Shell struct {
	// Allowlist holds command binaries that execute_shell_command runs without a
	// per-call approval prompt. commands not listed still run, but only after the
	// user approves them. base names only; args are never shell-expanded. empty
	// falls back to inarid's built-in default set.
	Allowlist []string `json:"allowlist"`
}

type Config struct {
	Socket         string         `json:"socket"`
	MemoryBudgetMB int            `json:"memory_budget_mb"`
	OllamaBaseURL  string         `json:"ollama_base_url"`
	DataDir        string         `json:"data_dir"`
	MCPConnectors  []MCPConnector `json:"mcp_connectors"`
	Models         Models         `json:"models"`
	Shell          Shell          `json:"shell"`
	Ollama         Ollama         `json:"ollama,omitempty"`
	Theme          string         `json:"theme,omitempty"`
	// Provider names the active entry in Endpoints; empty falls back to the legacy
	// single OllamaBaseURL. lets a user switch local backends without rebuilding.
	Provider string `json:"provider,omitempty"`
	// Endpoints holds named backend profiles (e.g. "ollama", "lmstudio", "llamacpp").
	Endpoints map[string]Endpoint `json:"endpoints,omitempty"`
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
	Shell:            Shell{Allowlist: []string{}}, // empty -> inarid's built-in default allowlist
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

// ActiveEndpoint resolves the backend the daemon should talk to. it prefers the
// profile named by Provider; if Provider is empty or names a missing profile it
// falls back to the legacy single OllamaBaseURL. a profile with an empty base_url
// inherits OllamaBaseURL, so partial profiles still work. found reports whether a
// named profile actually matched (false on fallback), so the caller can warn on a
// dangling Provider name.
func (c *Config) ActiveEndpoint() (Endpoint, bool) {
	if c.Provider != "" {
		if ep, ok := c.Endpoints[c.Provider]; ok {
			if ep.BaseURL == "" {
				ep.BaseURL = c.OllamaBaseURL
			}
			return ep, true
		}
	}
	return Endpoint{BaseURL: c.OllamaBaseURL}, false
}

// Save writes the config back to path with indented JSON.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
