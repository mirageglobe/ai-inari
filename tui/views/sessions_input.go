// sessions_input.go owns the Sessions view's input handlers: mouse (wheel scroll and
// click-to-select a table row) and hotkeys (open/create/delete/close popup).
// it does NOT own data/mutation handlers (sessions_data.go / sessions_mutations.go)
// or the Update dispatch (sessions.go).

package views

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (h Sessions) onMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			h.table.MoveUp(3)
		case tea.MouseButtonWheelDown:
			h.table.MoveDown(3)
		case tea.MouseButtonLeft:
			// topbar(1) + box border-top(1) + col-header(1) = first data row at Y=3.
			// table body occupies Y=[3, 3+tableHeight-1]; border-bottom at 3+tableHeight.
			const tableBodyY = 3
			tableBodyEndY := tableBodyY + h.tableHeight - 1
			if msg.Y >= tableBodyY && msg.Y <= tableBodyEndY {
				tableH := h.table.Height()
				cursor := h.table.Cursor()
				cursorVisRow := min(cursor, tableH)
				clickedVisIdx := msg.Y - tableBodyY
				newCursor := cursor + clickedVisIdx - cursorVisRow
				if newCursor >= 0 && newCursor < len(h.sessions) {
					h.table.SetCursor(newCursor)
				}
			}
		}
	}
	return h, nil
}

// handleKey processes a hotkey. it returns the updated view, an optional
// command, and whether the key was handled; unhandled keys (e.g. table
// navigation) fall through to the table's own Update in the caller. it always
// clears the transient infoMsg, so the caller must use the returned value even
// when the key was not handled.
func (h Sessions) handleKey(msg tea.KeyMsg) (Sessions, tea.Cmd, bool) {
	h.infoMsg = ""

	// filter mode: keystrokes edit the filter string and the table live-filters.
	// esc clears and exits; enter keeps the filter and returns to hotkey navigation.
	if h.filtering {
		switch msg.Type {
		case tea.KeyEsc:
			h.filter, h.filtering = "", false
			h.applyFilter()
			h.rebuildTable()
			h.table.SetCursor(0)
			return h, nil, true
		case tea.KeyEnter:
			h.filtering = false
			return h, nil, true
		case tea.KeyBackspace:
			if r := []rune(h.filter); len(r) > 0 {
				h.filter = string(r[:len(r)-1])
				h.applyFilter()
				h.rebuildTable()
				h.table.SetCursor(0)
			}
			return h, nil, true
		case tea.KeyRunes, tea.KeySpace:
			if msg.Type == tea.KeySpace {
				h.filter += " "
			} else {
				h.filter += string(msg.Runes)
			}
			h.applyFilter()
			h.rebuildTable()
			h.table.SetCursor(0)
			return h, nil, true
		case tea.KeyUp, tea.KeyDown:
			return h, nil, false // let the table move the cursor within the filtered list
		default:
			return h, nil, true // swallow other keys while filtering
		}
	}

	// hotkeys only: no text input, no slash commands in this view.
	switch msg.String() {
	case "/":
		h.filtering = true
		return h, nil, true
	case "enter":
		if !h.offline {
			idx := h.table.Cursor()
			if idx >= 0 && idx < len(h.sessions) {
				sess := h.sessions[idx]
				return h, func() tea.Msg {
					return SelectModelMsg{SessionID: sess.ID, SessionName: sess.Name, ModelName: sess.Model, CWD: sess.CWD, ContextChars: sess.ContextChars, NumCtxOverride: sess.NumCtxOverride, SystemPrompt: sess.SystemPrompt}
				}, true
			}
		}
	case "q", "esc":
		if h.modal {
			return h, func() tea.Msg { return CloseSessionsModalMsg{} }, true
		}
	case "a":
		if !h.offline {
			name := pickSessionName(h.usedNames())
			return h, createSessionCmd(h.client, name), true
		}
	case "x":
		if !h.offline {
			idx := h.table.Cursor()
			if idx >= 0 && idx < len(h.sessions) {
				id := h.sessions[idx].ID
				return h, deleteSessionCmd(h.client, id), true
			}
		}
	}
	return h, nil, false
}
