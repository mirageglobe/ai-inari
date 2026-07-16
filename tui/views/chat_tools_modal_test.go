package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestToolsModalClosesOnQAndEsc asserts the builtin-tools modal captures both q
// and esc to close, even though it overlays the focused chat input (a plain q
// would otherwise type into the message box).
func TestToolsModalClosesOnQAndEsc(t *testing.T) {
	keys := map[string]tea.KeyMsg{
		"q":   {Type: tea.KeyRunes, Runes: []rune("q")},
		"esc": {Type: tea.KeyEsc},
	}
	for name, key := range keys {
		c := Chat{showBuiltin: true}
		nc, _, handled := c.handleKey(key)
		if !handled {
			t.Fatalf("%s should be handled to close the tools modal", name)
		}
		if nc.showBuiltin {
			t.Fatalf("tools modal should close on %s", name)
		}
	}
}
