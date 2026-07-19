package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

// applyFilter matches name and model, case-insensitively; an empty filter shows
// everything and produces a fresh slice (not aliased to allSessions).
func TestSessionsApplyFilter(t *testing.T) {
	h := NewSessions(nil)
	h.allSessions = []ipc.SessionInfo{
		{ID: "1", Name: "jade fox"},
		{ID: "2", Name: "wise otter"},
		{ID: "3", Name: "amber fox", Model: "gemma4:e2b"},
	}

	h.applyFilter()
	if len(h.sessions) != 3 {
		t.Fatalf("no filter: want 3, got %d", len(h.sessions))
	}

	h.filter = "fox"
	h.applyFilter()
	if len(h.sessions) != 2 {
		t.Fatalf("filter 'fox': want 2, got %d", len(h.sessions))
	}

	// case-insensitive, and matches on the model column too.
	h.filter = "GEMMA"
	h.applyFilter()
	if len(h.sessions) != 1 || h.sessions[0].ID != "3" {
		t.Fatalf("filter 'GEMMA' (model match): want [id 3], got %+v", h.sessions)
	}

	// no match -> empty view, backing set untouched.
	h.filter = "zzz"
	h.applyFilter()
	if len(h.sessions) != 0 {
		t.Fatalf("filter 'zzz': want 0, got %d", len(h.sessions))
	}
	if len(h.allSessions) != 3 {
		t.Fatalf("allSessions must be untouched by filtering, got %d", len(h.allSessions))
	}
}

// [/] enters filter mode; typing narrows the list live; esc clears and exits.
func TestSessionsFilterKeys(t *testing.T) {
	slash := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	keyF := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
	esc := tea.KeyMsg{Type: tea.KeyEsc}

	h := NewSessions(nil)
	h.allSessions = []ipc.SessionInfo{{ID: "1", Name: "jade fox"}, {ID: "2", Name: "wise otter"}}
	h.applyFilter()
	h.rebuildTable()

	// "/" enters filter mode without touching the filter string.
	h, _, handled := h.handleKey(slash)
	if !handled || !h.filtering {
		t.Fatalf("'/' should enter filter mode; handled=%v filtering=%v", handled, h.filtering)
	}

	// typing 'f' builds the filter and narrows to the single fox.
	h, _, handled = h.handleKey(keyF)
	if !handled || h.filter != "f" {
		t.Fatalf("typing should build filter; handled=%v filter=%q", handled, h.filter)
	}
	if len(h.sessions) != 1 || h.sessions[0].Name != "jade fox" {
		t.Fatalf("filter 'f': want [jade fox], got %+v", h.sessions)
	}

	// esc clears the filter and exits filter mode.
	h, _, _ = h.handleKey(esc)
	if h.filtering || h.filter != "" {
		t.Fatalf("esc should clear+exit; filtering=%v filter=%q", h.filtering, h.filter)
	}
	if len(h.sessions) != 2 {
		t.Fatalf("after clear: want 2 sessions, got %d", len(h.sessions))
	}
}
