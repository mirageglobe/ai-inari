package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ProjectConfigPath is the fixed location of the per-project overlay, relative to
// a session's working directory.
const ProjectConfigPath = ".inari/config.json"

// ProjectConfig is the restricted per-project overlay read from
// <cwd>/.inari/config.json. only presentation/context fields are honored:
// infra and security fields (socket, endpoints, provider, shell.allowlist,
// models, data_dir, ollama, ...) are deliberately NOT part of this struct, so a
// project directory - which may be an untrusted cloned repo - cannot widen the
// shell allowlist or redirect the inference backend. any such keys present in the
// file are silently ignored by the JSON decoder.
type ProjectConfig struct {
	// Context.SystemPrompt, when set, replaces the global context.system_prompt in
	// the prepend slot for sessions opened in this directory (more-specific wins).
	Context Context `json:"context,omitempty"`
	// ExcludeDirs are extra directory names pruned from the injected file tree for
	// sessions in this directory, added to inarid's built-in skip set.
	ExcludeDirs []string `json:"exclude_dirs,omitempty"`
}

// LoadProject reads the restricted overlay from <cwd>/.inari/config.json. a
// missing or malformed file yields the zero value (no overlay), so callers apply
// it unconditionally without an error path. only the fields declared on
// ProjectConfig are decoded; every other key in the file is ignored.
func LoadProject(cwd string) ProjectConfig {
	if cwd == "" {
		return ProjectConfig{}
	}
	data, err := os.ReadFile(filepath.Join(cwd, ProjectConfigPath))
	if err != nil {
		return ProjectConfig{}
	}
	var pc ProjectConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		return ProjectConfig{} // malformed overlay is ignored, not fatal
	}
	return pc
}
