// agents_table.go owns the Agents view's table construction and session
// accessors: rebuildTable plus SelectedSession/DefaultSession/usedNames. it does
// NOT own rendering (agents_view.go), data/lifecycle handlers (agents_data.go),
// or the Update dispatch (agents.go).

package views

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

func (h *Agents) rebuildTable() {
	sort.Slice(h.sessions, func(i, j int) bool {
		return h.sessions[i].Name < h.sessions[j].Name
	})
	rows := make([]table.Row, len(h.sessions))
	for i, s := range h.sessions {
		vram, status := "-", "-"
		if info, ok := h.runningInfo[s.Model]; ok {
			vram = formatBytes(info.vram)
			status = formatExpiry(info.expiry)
		} else if s.Model != "" {
			// model assigned but not currently loaded in ollama memory
			status = "sleeping"
		}
		model := s.Model
		if model == "" {
			model = "-"
		} else if caps, ok := h.modelCaps[s.Model]; ok {
			for _, c := range caps {
				switch c {
				case "tools":
					model += " [tool]"
				case "vision":
					model += " [vis]"
				}
			}
		}
		ctx := "-"
		if s.ContextChars > 0 {
			ctx = fmtTokens(s.ContextChars)
		}
		indicator := " "
		if s.ID == h.activeSessionID {
			indicator = ">"
		}
		// append tags after the name so they are visible for grouping, e.g.
		// "jade fox [work urgent]".
		name := s.Name
		if len(s.Tags) > 0 {
			name += " [" + strings.Join(s.Tags, " ") + "]"
		}
		rows[i] = table.Row{indicator, name, model, vram, status, ctx}
	}
	h.table.SetRows(rows)
}

// SelectedSession returns the session at the current cursor plus its vram.
// returns false if no session is under the cursor.
func (h Agents) SelectedSession() (ipc.SessionInfo, int64, bool) {
	idx := h.table.Cursor()
	if idx < 0 || idx >= len(h.sessions) {
		return ipc.SessionInfo{}, 0, false
	}
	sess := h.sessions[idx]
	return sess, h.runningInfo[sess.Model].vram, true
}

// DefaultSession returns the first session in the list (by name, since the
// table is sorted alphabetically), used by chat's /chat command to jump back
// to the default agent regardless of which session is currently active.
func (h Agents) DefaultSession() (ipc.SessionInfo, bool) {
	if len(h.allSessions) == 0 {
		return ipc.SessionInfo{}, false
	}
	return h.allSessions[0], true
}

// usedNames returns every session name (the full set, not the filtered view) so
// pickAgentName never collides with a name hidden behind an active filter.
func (h Agents) usedNames() []string {
	names := make([]string, len(h.allSessions))
	for i, s := range h.allSessions {
		names[i] = s.Name
	}
	return names
}
