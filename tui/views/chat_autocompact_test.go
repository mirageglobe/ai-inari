package views

import "testing"

// TestShouldAutoCompact asserts the threshold decision against the effective
// context window: no compaction when the window is unknown, none below the
// fraction, and compaction at/above it. the estimate is ctxChars/4.
func TestShouldAutoCompact(t *testing.T) {
	// window 8192 tokens; 80% = 6553.6 tokens = ~26215 chars.
	const window = 8192

	cases := []struct {
		name     string
		ctxChars int
		window   int
		want     bool
	}{
		{"unknown window never compacts", 1 << 20, 0, false},
		{"well below threshold", 4000, window, false},  // ~1000 tokens
		{"just below threshold", 25000, window, false}, // ~6250 tokens
		{"above threshold", 28000, window, true},       // ~7000 tokens
		{"far above threshold", 40000, window, true},   // ~10000 tokens
	}
	for _, tc := range cases {
		if got := shouldAutoCompact(tc.ctxChars, tc.window); got != tc.want {
			t.Errorf("%s: shouldAutoCompact(%d, %d) = %v, want %v", tc.name, tc.ctxChars, tc.window, got, tc.want)
		}
	}
}

// TestEffectiveNumCtx asserts the override wins over the model-derived default,
// and that with no override the capped default (DefaultNumCtx) applies.
func TestEffectiveNumCtx(t *testing.T) {
	cases := []struct {
		name           string
		numCtxOverride int
		maxCtx         int
		want           int
	}{
		{"override wins over computed default", 4096, 40960, 4096},
		{"override wins even when max unknown", 2048, 0, 2048},
		{"no override uses capped default", 0, 40960, 8192}, // DefaultNumCtx caps at 8192
		{"no override, small max uses max", 0, 4000, 4000},
		{"no override, unknown max is zero", 0, 0, 0},
	}
	for _, tc := range cases {
		if got := effectiveNumCtx(tc.numCtxOverride, tc.maxCtx); got != tc.want {
			t.Errorf("%s: effectiveNumCtx(%d, %d) = %d, want %d", tc.name, tc.numCtxOverride, tc.maxCtx, got, tc.want)
		}
	}
}
