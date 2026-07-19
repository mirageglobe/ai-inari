package views

import (
	"fmt"
	"strings"
	"testing"
)

// pickSessionName returns an adjective+noun name from the pools, never reuses a
// name already in use, and falls back to a numbered session once exhausted.
func TestPickSessionName(t *testing.T) {
	adjs := make(map[string]bool, len(sessionAdjectives))
	for _, a := range sessionAdjectives {
		adjs[a] = true
	}
	nouns := make(map[string]bool, len(sessionNouns))
	for _, n := range sessionNouns {
		nouns[n] = true
	}

	// fresh name: two words, each drawn from the pools.
	name := pickSessionName(nil)
	parts := strings.SplitN(name, " ", 2)
	if len(parts) != 2 || !adjs[parts[0]] || !nouns[parts[1]] {
		t.Fatalf("pickSessionName(nil) = %q, want <adjective> <noun> from the pools", name)
	}

	// dedup: a returned name is never one already in use.
	used := []string{name}
	for i := 0; i < 50; i++ {
		got := pickSessionName(used)
		for _, u := range used {
			if got == u {
				t.Fatalf("pickSessionName returned in-use name %q", got)
			}
		}
		used = append(used, got)
	}

	// exhaustion: with every combination taken, fall back to a numbered session.
	all := make([]string, 0, len(sessionAdjectives)*len(sessionNouns))
	for _, a := range sessionAdjectives {
		for _, n := range sessionNouns {
			all = append(all, a+" "+n)
		}
	}
	got := pickSessionName(all)
	if want := fmt.Sprintf("session #%d", len(all)+1); got != want {
		t.Errorf("exhausted pool: pickSessionName = %q, want %q", got, want)
	}
}

// every built-in theme has a name and a populated Ray gradient; names are unique.
func TestThemesWellFormed(t *testing.T) {
	seen := make(map[string]bool, len(Themes))
	for _, th := range Themes {
		if th.Name == "" {
			t.Errorf("theme with empty name: %+v", th)
		}
		if seen[th.Name] {
			t.Errorf("duplicate theme name %q", th.Name)
		}
		seen[th.Name] = true
		for i, c := range th.Ray {
			if c == "" {
				t.Errorf("theme %q Ray[%d] is empty", th.Name, i)
			}
		}
	}
	for _, want := range []string{"emerald", "cyan", "mono"} {
		if !seen[want] {
			t.Errorf("expected new theme %q in Themes", want)
		}
	}
}
