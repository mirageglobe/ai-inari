package ipc

import "testing"

// TestHasRepeatedTail asserts the runaway-loop detector trips on a short sequence
// repeated 3+ times at the tail, but not on normal prose, punctuation runs, or
// zero-padded numbers (which are legitimate).
func TestHasRepeatedTail(t *testing.T) {
	trips := []string{
		"warming up... for_for_for_for",
		"abcabcabcabc",
		"reply: bye bye bye bye ",
	}
	for _, s := range trips {
		if !hasRepeatedTail(s) {
			t.Errorf("expected loop detected in %q", s)
		}
	}

	clean := []string{
		"",
		"the quick brown fox jumps over",
		"--------------------",
		"pi is 3.14159265358979000000",
		"one two three four five",
	}
	for _, s := range clean {
		if hasRepeatedTail(s) {
			t.Errorf("false positive on %q", s)
		}
	}
}
