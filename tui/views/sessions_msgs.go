// sessions_msgs.go owns the Sessions view's internal message types and the
// runningMeta helper struct. it does NOT own the tea.Cmd constructors that
// produce these messages (sessions_cmds.go) or naming/formatting helpers (sessions_fmt.go).

package views

import (
	"github.com/mirageglobe/inari/internal/ipc"
	"github.com/mirageglobe/inari/internal/provider"
)

// runningMeta holds live stats for a running model, used to populate VRAM/Status columns.
type runningMeta struct {
	vram   int64
	expiry string
}

// OpenLogsMsg is emitted by chat's /logs command to navigate to the logs view.
type OpenLogsMsg struct{}

// OpenDescribeMsg is emitted by chat's /describe command to navigate to the describe view.
type OpenDescribeMsg struct{}

// CycleThemeMsg is emitted by chat's /theme command to cycle to the next theme.
type CycleThemeMsg struct{}

// ToggleHelpMsg is emitted by chat's /help command to open/close the help overlay.
type ToggleHelpMsg struct{}

// OpenSessionsMsg is emitted by chat's /sessions command to open sessions as a popup
// modal over chat, rather than switching away from chat entirely.
type OpenSessionsMsg struct{}

// CloseSessionsModalMsg is emitted by sessions when [q] is pressed while it is
// rendered as a popup modal, returning the root model to chat.
type CloseSessionsModalMsg struct{}

// OpenDefaultChatMsg is emitted by chat's /chat command to jump to the
// default session's chat regardless of which session is currently active.
type OpenDefaultChatMsg struct{}

// RefreshSessionsMsg is emitted by chat's /refresh command to silently reload
// the sessions session list and running-model info in the background.
type RefreshSessionsMsg struct{}

type runningMsg struct {
	models []provider.RunningModel
	err    error
}

type sessionsMsg struct {
	sessions []ipc.SessionInfo
	err      error
}

type createSessionResultMsg struct {
	session ipc.SessionInfo
	err     error
}

type deleteSessionResultMsg struct {
	id  string
	err error
}

type assignModelResultMsg struct {
	id  string
	err error
}

type unassignModelResultMsg struct {
	id  string
	err error
}

type exportChatResultMsg struct {
	path string
	err  error
}

type modelCapsMsg struct {
	model string
	caps  []string
}
