package ipc

import (
	"strings"
	"testing"
)

// TestHasRepeatedTailIgnoresLongPrefix asserts the detector still fires on a
// looping tail after a large prefix (and stays quiet without one), proving the
// slice-before-convert change inspects only the tail regardless of total length.
func TestHasRepeatedTailIgnoresLongPrefix(t *testing.T) {
	// 10k-byte prefix + a period-3 block repeated 4x at the very end.
	looping := strings.Repeat("x", 10000) + strings.Repeat("foo", 4)
	if !hasRepeatedTail(looping) {
		t.Fatal("should detect a period-3 loop at the tail after a long prefix")
	}

	// same-size string with no repeated block at the end.
	clean := strings.Repeat("x", 10000) + "abcdefghijklmnop"
	if hasRepeatedTail(clean) {
		t.Fatal("a non-looping tail should not trip the detector")
	}
}
