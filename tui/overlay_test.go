package tui

import "testing"

// TestTopOverlayPriority locks in the priority order and, crucially, the one nesting
// a flat enum could not represent: the model selector sits ON TOP of the sessions
// popup, and closing the selector reveals the popup underneath.
func TestTopOverlayPriority(t *testing.T) {
	m := New(nil, "", 0)
	if m.topOverlay() != overlayNone {
		t.Fatalf("fresh model should have no overlay, got %d", m.topOverlay())
	}

	// selector opened over the sessions popup: both bools set, selector wins.
	m.showSessions = true
	m.showModelSelector = true
	if m.topOverlay() != overlayModelSelector {
		t.Fatalf("selector should outrank the sessions popup, got %d", m.topOverlay())
	}

	// closing the selector must reveal the sessions popup, not fall through to chat.
	m.showModelSelector = false
	if m.topOverlay() != overlaySessions {
		t.Fatalf("closing the selector should reveal the sessions popup, got %d", m.topOverlay())
	}
}
