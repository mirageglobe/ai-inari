package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/inari/tui/views"
)

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// chatBaseModel returns a root model sitting on an active chat, so overlays open
// "over chat" and closing them should reveal chat again.
func chatBaseModel() Model {
	m := New(nil, "", 0)
	m.current = viewChat
	m.activeSession = "s"
	m.chats["s"] = views.NewChat(nil, "s", "sess", "model", "/tmp", 0, 0, "")
	return m
}

// TestModalsCloseOnBothQAndEsc asserts every read-only overlay opens from its
// message and closes on BOTH q and esc, returning to the view underneath (chat).
func TestModalsCloseOnBothQAndEsc(t *testing.T) {
	keys := map[string]tea.KeyMsg{"q": keyRune('q'), "esc": {Type: tea.KeyEsc}}
	cases := []struct {
		name   string
		open   tea.Msg
		isOpen func(Model) bool
	}{
		{"describe", views.OpenDescribeMsg{}, func(m Model) bool { return m.showDescribe }},
		{"logs", views.OpenLogsMsg{}, func(m Model) bool { return m.showLogs }},
		{"help", views.ToggleHelpMsg{}, func(m Model) bool { return m.showHelp }},
		{"theme", views.CycleThemeMsg{}, func(m Model) bool { return m.showThemePicker }},
	}
	for _, tc := range cases {
		for keyName, key := range keys {
			t.Run(tc.name+"/"+keyName, func(t *testing.T) {
				m := chatBaseModel()
				opened, _ := m.Update(tc.open)
				m = opened.(Model)
				if !tc.isOpen(m) {
					t.Fatalf("%s should be open after its open message", tc.name)
				}
				closed, _ := m.Update(key)
				m = closed.(Model)
				if tc.isOpen(m) {
					t.Fatalf("%s should close on %q", tc.name, keyName)
				}
				if m.current != viewChat {
					t.Fatalf("closing %s should reveal the underlying chat view, got current=%d", tc.name, m.current)
				}
			})
		}
	}
}

// TestOverlayReturnsToViewUnderneath asserts a modal opened from sessions returns to
// sessions on close (not forced to chat).
func TestOverlayReturnsToViewUnderneath(t *testing.T) {
	m := New(nil, "", 0) // base view is sessions
	opened, _ := m.Update(views.OpenLogsMsg{})
	m = opened.(Model)
	if !m.showLogs || m.current != viewSessions {
		t.Fatalf("logs should open over sessions, got showLogs=%v current=%d", m.showLogs, m.current)
	}
	closed, _ := m.Update(keyRune('q'))
	m = closed.(Model)
	if m.showLogs || m.current != viewSessions {
		t.Fatalf("closing logs should return to sessions, got showLogs=%v current=%d", m.showLogs, m.current)
	}
}
