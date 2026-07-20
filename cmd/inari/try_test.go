package main

import "testing"

// TestRegistryManifestURL covers tag -> registry manifest URL mapping: bare library
// models get the library/ prefix, a missing tag defaults to latest, and namespaced
// (user/model) tags are used as-is.
func TestRegistryManifestURL(t *testing.T) {
	cases := []struct {
		tag  string
		want string
	}{
		{"llama3.2:3b", "https://registry.ollama.ai/v2/library/llama3.2/manifests/3b"},
		{"gemma4", "https://registry.ollama.ai/v2/library/gemma4/manifests/latest"},
		{"qwen3.6:27b-coding-nvfp4", "https://registry.ollama.ai/v2/library/qwen3.6/manifests/27b-coding-nvfp4"},
		{"acme/custom:v1", "https://registry.ollama.ai/v2/acme/custom/manifests/v1"},
		{"acme/custom", "https://registry.ollama.ai/v2/acme/custom/manifests/latest"},
	}
	for _, tc := range cases {
		if got := registryManifestURL(tc.tag); got != tc.want {
			t.Errorf("registryManifestURL(%q) = %q, want %q", tc.tag, got, tc.want)
		}
	}
}
