// agents_fmt.go owns session-name generation and byte/duration formatting
// helpers shared across the agents, selector, and describe views. it does NOT
// own message types (agents_msgs.go) or tea.Cmd constructors (agents_cmds.go).

package views

import (
	"fmt"
	"math/rand"
	"time"
)

// sessionAdjectives are paired with "agent" to form session names like "jade agent".
var sessionAdjectives = []string{
	"arctic", "amber", "ash", "blaze", "copper", "crimson", "dusk",
	"ember", "fire", "frost", "ghost", "golden", "jade", "midnight",
	"rusty", "scarlet", "shadow", "silver", "storm", "swift", "thunder",
	"tundra", "violet", "wild",
}

// pickAgentName returns an agent-themed name not already in use.
func pickAgentName(used []string) string {
	inUse := make(map[string]bool, len(used))
	for _, v := range used {
		inUse[v] = true
	}
	pool := make([]string, len(sessionAdjectives))
	copy(pool, sessionAdjectives)
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	for _, adj := range pool {
		name := adj + " agent"
		if !inUse[name] {
			return name
		}
	}
	return fmt.Sprintf("agent #%d", len(used)+1)
}

// formatBytes formats a byte count as a human-readable string (GB/MB/B).
func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(b)/float64(1<<20))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// formatExpiry formats an RFC3339 expiry timestamp as a human-readable countdown.
func formatExpiry(expiresAt string) string {
	t, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return "—"
	}
	d := time.Until(t).Round(time.Second)
	if d <= 0 {
		return "waking"
	}
	return fmt.Sprintf("in %s", d)
}
