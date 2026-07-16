package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/tui/views"
)

// TestIsBareKey asserts modifier chords are not bare while plain keys are.
func TestIsBareKey(t *testing.T) {
	bare := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("?")},
		{Type: tea.KeyRunes, Runes: []rune("t")},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEsc},
		{Type: tea.KeyUp},
	}
	for _, k := range bare {
		if !isBareKey(k) {
			t.Errorf("expected %q to be a bare key", k.String())
		}
	}
	for _, k := range []tea.KeyMsg{{Type: tea.KeyCtrlO}, {Type: tea.KeyCtrlP}} {
		if isBareKey(k) {
			t.Errorf("expected %q to NOT be a bare key", k.String())
		}
	}
}

// TestFocusAwareKeySuppression asserts that while a view's text input is focused,
// bare keys fall through to the view (not handled globally), while modifier chords
// are still handled, and that with no input focused a global bare key is handled.
func TestFocusAwareKeySuppression(t *testing.T) {
	m := New(nil, "", 0)
	m.current = viewChat
	m.activeSession = "s"
	m.chats["s"] = views.NewChat(nil, "s", "sess", "model", "", 0, 0, "") // InputFocused() == true

	// bare key while the chat input is focused: suppressed (falls through).
	if _, _, handled := m.updateKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")}); handled {
		t.Fatal("bare key should fall through to the view when an input is focused")
	}
	// modifier chord still reaches the per-view global binding (chat ctrl+o).
	if _, _, handled := m.updateKeys(tea.KeyMsg{Type: tea.KeyCtrlO}); !handled {
		t.Fatal("ctrl+o should still be handled while the chat input is focused")
	}
}
