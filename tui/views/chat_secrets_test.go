package views

import "testing"

// TestLooksLikeSecret asserts the heuristic flags credential-shaped strings
// (known token prefixes, key=value assignments) but not ordinary prose that
// merely mentions keys/tokens.
func TestLooksLikeSecret(t *testing.T) {
	secrets := []string{
		"my key is sk-abcdef123456",
		"export GITHUB_TOKEN=ghp_abcdefghij",
		"api_key: 1234567890abcdef",
		"password = hunter2xyz",
		"AKIA1234567890ABCD",
	}
	for _, s := range secrets {
		if !looksLikeSecret(s) {
			t.Errorf("expected secret detected in %q", s)
		}
	}

	clean := []string{
		"how do i rotate my api key safely?",
		"what does rm -rf do?",
		"tell me about tokens in nlp",
		"",
		"just a normal message",
	}
	for _, s := range clean {
		if looksLikeSecret(s) {
			t.Errorf("false positive on %q", s)
		}
	}
}
