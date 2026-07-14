// chat_types.go owns the chat view's shared lipgloss styles and the message
// types exchanged with the root model (streamed tokens, done/status signals,
// history/context/recap loads). it does NOT own the Chat struct (chat.go), the
// Update dispatcher (chat_update.go), or any handler logic.

package views

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

var (
	userStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	thinkingStyle  = lipgloss.NewStyle().Faint(true)
)

// ChatTokenMsg is sent for each streamed token from inarid.
// SessionID routes it to the correct Chat view regardless of which view is active.
type ChatTokenMsg struct {
	SessionID string
	Token     string
}

// ChatDoneMsg is sent when the stream ends (success or error).
type ChatDoneMsg struct {
	SessionID string
	Err       error
}

// ChatStatusMsg carries a coarse phase signal from inarid: "loading" while the
// model is being cold-loaded into backend memory, "thinking" once generation
// has actually begun. absent entirely when the model was already resident.
type ChatStatusMsg struct {
	SessionID string
	Status    string
}

type chatHistoryMsg struct {
	messages []provider.Message
	err      error
}

// modelContextMsg carries the assigned model's maximum context window (tokens),
// fetched once on chat open; 0 when unknown.
type modelContextMsg struct{ max int }

// recapMsg carries a one-line "where you left off" summary for an idle session,
// fetched on open; empty when the session is not idle or has nothing to recap.
type recapMsg struct{ text string }
