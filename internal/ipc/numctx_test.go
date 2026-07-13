package ipc

import "testing"

// DefaultNumCtx caps the model's max window at 8192 and returns 0 for an unknown
// (<= 0) max so callers omit the option and fall back to the backend default.
func TestDefaultNumCtx(t *testing.T) {
	cases := []struct{ max, want int }{
		{0, 0},        // unknown -> omit
		{-5, 0},       // guard against negatives
		{2048, 2048},  // below cap -> as-is
		{8192, 8192},  // at cap
		{40960, 8192}, // above cap -> capped
	}
	for _, c := range cases {
		if got := DefaultNumCtx(c.max); got != c.want {
			t.Errorf("DefaultNumCtx(%d) = %d, want %d", c.max, got, c.want)
		}
	}
}
