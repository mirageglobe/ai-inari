// chat_secrets.go owns the client-side pre-send secret heuristic: a soft,
// non-blocking warning shown when an outgoing chat message looks like it carries
// a credential. it does NOT block the send (that stays the user's call) and is
// distinct from the daemon-side low-effort short-circuit in handleStream.

package views

import (
	"regexp"
	"strings"
)

// secretAssignRe matches "<key-ish> = <value>" assignments that commonly carry
// credentials. case-insensitive; requires a non-trivial value so prose does not trip.
var secretAssignRe = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|passwd|bearer)\s*[:=]\s*\S{6,}`)

// secretPrefixes are well-known credential token prefixes; a substring match is a
// strong signal regardless of surrounding text.
var secretPrefixes = []string{"sk-", "ghp_", "gho_", "github_pat_", "xoxb-", "xoxp-", "AKIA", "AIza"}

// looksLikeSecret reports whether text likely contains a credential, so the UI can
// warn before sending it to the model. tuned for low false positives: a key=value
// assignment with a real value, or a known token prefix.
func looksLikeSecret(text string) bool {
	if secretAssignRe.MatchString(text) {
		return true
	}
	for _, p := range secretPrefixes {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}
