package config

import (
	"testing"
	"time"
)

// TestIdleTimeout asserts the 0->30min default, negative->disabled, positive->minutes
// resolution now lives in config.
func TestIdleTimeout(t *testing.T) {
	cases := []struct {
		mins int
		want time.Duration
	}{
		{0, 30 * time.Minute}, // unset / config predating the field
		{-1, 0},               // disabled
		{5, 5 * time.Minute},  // explicit
	}
	for _, c := range cases {
		got := (&Config{IdleShutdownMins: c.mins}).IdleTimeout()
		if got != c.want {
			t.Errorf("IdleShutdownMins=%d: got %v, want %v", c.mins, got, c.want)
		}
	}
}
