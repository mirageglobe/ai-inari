package config

import "testing"

// TestActiveEndpoint covers the legacy fallback, a named profile, base_url
// inheritance for a partial profile, and a dangling provider name.
func TestActiveEndpoint(t *testing.T) {
	t.Run("no provider falls back to ollama_base_url", func(t *testing.T) {
		c := &Config{OllamaBaseURL: "http://localhost:11434"}
		ep, named := c.ActiveEndpoint()
		if named || ep.BaseURL != "http://localhost:11434" {
			t.Fatalf("fallback: named=%v url=%q", named, ep.BaseURL)
		}
	})

	t.Run("named profile resolves with auth", func(t *testing.T) {
		c := &Config{
			OllamaBaseURL: "http://localhost:11434",
			Provider:      "lmstudio",
			Endpoints:     map[string]Endpoint{"lmstudio": {BaseURL: "http://localhost:1234", APIKey: "k"}},
		}
		ep, named := c.ActiveEndpoint()
		if !named || ep.BaseURL != "http://localhost:1234" || ep.APIKey != "k" {
			t.Fatalf("named profile: named=%v ep=%+v", named, ep)
		}
	})

	t.Run("partial profile inherits base_url", func(t *testing.T) {
		c := &Config{
			OllamaBaseURL: "http://localhost:11434",
			Provider:      "p",
			Endpoints:     map[string]Endpoint{"p": {APIKey: "k"}},
		}
		ep, named := c.ActiveEndpoint()
		if !named || ep.BaseURL != "http://localhost:11434" {
			t.Fatalf("empty base_url should inherit, got named=%v url=%q", named, ep.BaseURL)
		}
	})

	t.Run("dangling provider name falls back", func(t *testing.T) {
		c := &Config{OllamaBaseURL: "http://x", Provider: "missing", Endpoints: map[string]Endpoint{}}
		ep, named := c.ActiveEndpoint()
		if named || ep.BaseURL != "http://x" {
			t.Fatalf("dangling: named=%v url=%q", named, ep.BaseURL)
		}
	})
}
