package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	f, err := os.CreateTemp("", "haniwa-config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString(`{
		"socket": "/tmp/test.sock",
		"memory_budget_mb": 4096,
		"ollama_base_url": "http://localhost:11434",
		"mcp_connectors": [],
		"models": {"thinker": "bonsai:8b", "worker": "bonsai:4b", "sensor": "qwen3-nano"}
	}`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Socket != "/tmp/test.sock" {
		t.Errorf("socket = %q, want /tmp/test.sock", cfg.Socket)
	}
	if cfg.MemoryBudgetMB != 4096 {
		t.Errorf("memory_budget_mb = %d, want 4096", cfg.MemoryBudgetMB)
	}
	if cfg.Models.Thinker != "bonsai:8b" {
		t.Errorf("thinker = %q, want bonsai:8b", cfg.Models.Thinker)
	}
}
