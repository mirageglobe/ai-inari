package views

import (
	"testing"
	"time"
)

// TestOnIdleHintTick covers the idle-hint rotation: nothing before the delay,
// the first hint after one delay, advancing by elapsed idle time, wrap-around,
// and suppression while another status-line element is active.
func TestOnIdleHintTick(t *testing.T) {
	n := len(idleHintPool)
	if n == 0 {
		t.Fatal("idleHintPool must not be empty")
	}

	cases := []struct {
		name  string
		setup func(c Chat) Chat
		want  string
	}{
		{
			name:  "not idle yet",
			setup: func(c Chat) Chat { c.lastActivity = time.Now(); return c },
			want:  "",
		},
		{
			name:  "first hint after delay",
			setup: func(c Chat) Chat { c.lastActivity = time.Now().Add(-idleHintDelay - time.Second); return c },
			want:  idleHintPool[0],
		},
		{
			name:  "advances by elapsed idle time",
			setup: func(c Chat) Chat { c.lastActivity = time.Now().Add(-2*idleHintDelay - time.Second); return c },
			want:  idleHintPool[1%n],
		},
		{
			name: "wraps around the pool",
			setup: func(c Chat) Chat {
				c.lastActivity = time.Now().Add(-time.Duration(n+1)*idleHintDelay - time.Second)
				return c
			},
			want: idleHintPool[n%n], // == pool[0]
		},
		{
			name: "suppressed while a status is shown",
			setup: func(c Chat) Chat {
				c.lastActivity = time.Now().Add(-idleHintDelay - time.Second)
				c.status = "[recap] earlier"
				return c
			},
			want: "",
		},
		{
			name: "suppressed while waiting on a reply",
			setup: func(c Chat) Chat {
				c.lastActivity = time.Now().Add(-idleHintDelay - time.Second)
				c.waiting = true
				return c
			},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.setup(Chat{})
			got, _ := c.onIdleHintTick()
			if h := got.(Chat).idleHint; h != tc.want {
				t.Fatalf("idleHint = %q, want %q", h, tc.want)
			}
		})
	}
}
