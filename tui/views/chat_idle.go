// chat_idle.go owns the chat view's idle usage-hint feature: the static hint
// pool, the poll tick, and the tick handler that surfaces a rotating "how to use
// inari" line once the chat has been idle. it does NOT own the status-line render
// (chat_render.go) or activity tracking resets (chat.go / chat_stream.go).

package views

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// idleHintDelay is how long the chat must sit idle before the first hint shows;
// the shown hint then advances to the next entry every idleHintDelay thereafter.
const idleHintDelay = 60 * time.Second

// idleHintPoll is the cadence of the root-owned idle tick. kept below
// idleHintDelay so the first hint appears within one poll of the idle threshold.
const idleHintPoll = 20 * time.Second

// idleHintPool is the static rotation of usage hints, drawn in order by elapsed
// idle time. every entry names a real binding (see chat_keys.go / chat_commands.go).
var idleHintPool = []string{
	"try /compact to summarise a long chat",
	"ctrl+p opens the slash command palette",
	"/model assigns or unloads a model",
	"up / down recalls your previous messages",
	"/copy puts the last reply on your clipboard",
	"tab completes a slash command",
	"/export saves the full conversation to a file",
	"/theme cycles the colour theme",
}

// IdleHintTickMsg is the root-owned poll that drives idle-hint rotation. the root
// broadcasts it to every chat and reschedules IdleHintTick on receipt.
type IdleHintTickMsg struct{}

// IdleHintTick fires IdleHintTickMsg after idleHintPoll. one loop is started from
// the root model's Init so no per-chat Init can double-start it.
func IdleHintTick() tea.Cmd {
	return tea.Tick(idleHintPoll, func(_ time.Time) tea.Msg {
		return IdleHintTickMsg{}
	})
}

// onIdleHintTick updates the idle hint from the time since the last activity.
// it clears the hint whenever another element owns the status line or a
// stream/tool is active, so hints never clobber a recap, error, or live reply.
func (c Chat) onIdleHintTick() (tea.Model, tea.Cmd) {
	busy := c.waiting || c.status != "" || c.pendingTool != nil ||
		c.runningTool != "" || c.streamBuf != ""
	idleFor := time.Since(c.lastActivity)
	if busy || idleFor < idleHintDelay || len(idleHintPool) == 0 {
		c.idleHint = ""
		return c, nil
	}
	// idx advances one entry per completed idleHintDelay of idleness.
	idx := int(idleFor/idleHintDelay) - 1
	c.idleHint = idleHintPool[idx%len(idleHintPool)]
	return c, nil
}
