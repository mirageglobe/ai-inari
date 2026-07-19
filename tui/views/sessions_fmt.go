// sessions_fmt.go owns session-name generation and byte/duration formatting
// helpers shared across the sessions, selector, and describe views. it does NOT
// own message types (sessions_msgs.go) or tea.Cmd constructors (sessions_cmds.go).

package views

import (
	"fmt"
	"math/rand"
	"time"
)

// sessionAdjectives and sessionNouns are paired to form session names like
// "jade fox". pairing (not a fixed "session" suffix) gives N*M combinations, so
// name generation only exhausts after len(adj)*len(noun) sessions.
var sessionAdjectives = []string{
	"arctic", "amber", "ash", "blaze", "copper", "crimson", "dusk",
	"ember", "fire", "frost", "ghost", "golden", "jade", "midnight",
	"rusty", "scarlet", "shadow", "silver", "storm", "swift", "thunder",
	"tundra", "violet", "wild",
}

// sessionNouns keep inari's fox/woodland flavour (kitsune are the messengers).
var sessionNouns = []string{
	"fox", "otter", "lynx", "heron", "crane", "raven", "marten", "wren",
	"badger", "ferret", "gecko", "hare", "lemur", "magpie", "meerkat",
	"newt", "ocelot", "osprey", "puffin", "quokka", "robin", "sable",
	"stoat", "tapir", "vole", "weasel",
}

// pickSessionName returns an adjective+noun session name not already in use,
// falling back to a numbered session once every combination is taken.
func pickSessionName(used []string) string {
	inUse := make(map[string]bool, len(used))
	for _, v := range used {
		inUse[v] = true
	}
	adjs := make([]string, len(sessionAdjectives))
	copy(adjs, sessionAdjectives)
	rand.Shuffle(len(adjs), func(i, j int) { adjs[i], adjs[j] = adjs[j], adjs[i] })
	nouns := make([]string, len(sessionNouns))
	copy(nouns, sessionNouns)
	rand.Shuffle(len(nouns), func(i, j int) { nouns[i], nouns[j] = nouns[j], nouns[i] })
	for _, adj := range adjs {
		for _, noun := range nouns {
			name := adj + " " + noun
			if !inUse[name] {
				return name
			}
		}
	}
	return fmt.Sprintf("session #%d", len(used)+1)
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
