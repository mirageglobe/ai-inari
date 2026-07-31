// model pull progress plumbing for the model selector: message types and the
// channel-polling tea.Cmd that streams provider.PullProgress updates into the
// Bubble Tea event loop. selector.go owns triggering the pull and rendering it.

package views

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/inari/internal/provider"
)

// pullProgressMsg carries one status update from an in-progress model pull.
type pullProgressMsg struct {
	model    string
	progress provider.PullProgress
}

// pullDoneMsg is sent when the pull stream ends (success or error).
type pullDoneMsg struct {
	model string
	err   error
}

// readNextPullUpdate returns a cmd that blocks until the next progress update
// or the channel closes, then emits pullProgressMsg or pullDoneMsg.
func readNextPullUpdate(model string, progress <-chan provider.PullProgress, errc <-chan error) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-progress
		if !ok {
			return pullDoneMsg{model: model, err: <-errc}
		}
		return pullProgressMsg{model: model, progress: p}
	}
}

// pullStatusText formats a progress update for the selector's status line.
func pullStatusText(model string, p provider.PullProgress) string {
	if p.Status == "downloading" && p.Total > 0 {
		pct := p.Completed * 100 / p.Total
		return fmt.Sprintf("pulling %s... %d%%", model, pct)
	}
	return fmt.Sprintf("pulling %s: %s", model, p.Status)
}
