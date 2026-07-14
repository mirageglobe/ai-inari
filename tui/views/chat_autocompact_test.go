package views

import "testing"

// TestShouldAutoCompact asserts the threshold decision: no compaction when the
// window is unknown, none below the fraction, and compaction at/above it. the
// window is DefaultNumCtx(maxCtx) and the estimate is ctxChars/4.
func TestShouldAutoCompact(t *testing.T) {
	// maxCtx 40960 -> effective window capped at 8192 tokens -> 80% = 6553 tokens
	// -> ~26214 chars. use maxCtx well above the 8192 cap.
	const maxCtx = 40960

	// effective window = 8192 tokens; 80% = 6553.6 tokens = ~26215 chars.
	cases := []struct {
		name     string
		ctxChars int
		maxCtx   int
		want     bool
	}{
		{"unknown window never compacts", 1 << 20, 0, false},
		{"well below threshold", 4000, maxCtx, false},  // ~1000 tokens
		{"just below threshold", 25000, maxCtx, false}, // ~6250 tokens
		{"above threshold", 28000, maxCtx, true},       // ~7000 tokens
		{"far above threshold", 40000, maxCtx, true},   // ~10000 tokens
	}
	for _, tc := range cases {
		if got := shouldAutoCompact(tc.ctxChars, tc.maxCtx); got != tc.want {
			t.Errorf("%s: shouldAutoCompact(%d, %d) = %v, want %v", tc.name, tc.ctxChars, tc.maxCtx, got, tc.want)
		}
	}
}
