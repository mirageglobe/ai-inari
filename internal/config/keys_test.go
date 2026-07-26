package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// the json decoder drops keys no field accepts, so a config carrying a renamed or
// misspelled field looks configured while doing nothing. measured on a live config
// still holding the pre-consolidation models.worker/models.sensor.
func TestUnknownKeysFindsStaleFields(t *testing.T) {
	path := writeCfg(t, `{
	  "socket": "/tmp/x.sock",
	  "models": {"thinker": "gemma4:e2b", "worker": "bonsai:4b", "sensor": "qwen3-nano"},
	  "typo_at_root": 1
	}`)
	got, err := UnknownKeys(path)
	if err != nil {
		t.Fatalf("UnknownKeys: %v", err)
	}
	want := []string{"models.sensor", "models.worker", "typo_at_root"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v want %v", got, want)
	}
}

// a config using only real fields must stay silent, including the nested blocks,
// or the warning becomes noise the user learns to ignore.
func TestUnknownKeysCleanConfig(t *testing.T) {
	path := writeCfg(t, `{
	  "socket": "/tmp/x.sock",
	  "models": {"thinker": "gemma4:e2b", "runner": "gemma4:e4b"},
	  "context": {"system_prompt": "be terse"},
	  "ollama": {"keep_alive": "5m"},
	  "shell": {"allowlist": ["go", "make"]}
	}`)
	got, err := UnknownKeys(path)
	if err != nil {
		t.Fatalf("UnknownKeys: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("clean config reported %v", got)
	}
}

// endpoint profile names are user-chosen map keys, so they are not unknown fields;
// a typo inside a profile still is.
func TestUnknownKeysMapKeysAreNotFields(t *testing.T) {
	path := writeCfg(t, `{
	  "endpoints": {"lmstudio": {"base_url": "http://localhost:1234", "base_urll": "x"}}
	}`)
	got, err := UnknownKeys(path)
	if err != nil {
		t.Fatalf("UnknownKeys: %v", err)
	}
	want := []string{"endpoints.lmstudio.base_urll"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v want %v", got, want)
	}
}

// a missing or unreadable file is not a validation failure: Load already reports
// that, and doctor must not print the same problem twice.
func TestUnknownKeysMissingFile(t *testing.T) {
	got, err := UnknownKeys(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil || got != nil {
		t.Errorf("got %v, %v; want nil, nil", got, err)
	}
}
