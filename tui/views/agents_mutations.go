// agents_mutations.go owns the Agents view's mutation-result handlers: the
// outcomes of create/delete/assign/unassign/export RPCs and the optimistic
// AssignModelMsg update. it does NOT own data/lifecycle handlers
// (agents_data.go), input (agents_input.go), or the Update dispatch (agents.go).

package views

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (h Agents) onCreate(msg createSessionResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		h.status = connErrStyle.Render("create failed: " + msg.err.Error())
		return h, nil
	}
	return h, fetchSessions(h.client)
}

// setSessionModel updates a session's model in the backing list and refreshes
// the filtered view + table, so an active filter stays consistent.
func (h *Agents) setSessionModel(id, model string) {
	for i := range h.allSessions {
		if h.allSessions[i].ID == id {
			h.allSessions[i].Model = model
			break
		}
	}
	h.applyFilter()
	h.rebuildTable()
}

func (h Agents) onDelete(msg deleteSessionResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		h.status = connErrStyle.Render("delete failed: " + msg.err.Error())
		return h, nil
	}
	// note the displayed row of the deleted session for cursor repositioning.
	deletedIdx := -1
	for i, s := range h.sessions {
		if s.ID == msg.id {
			deletedIdx = i
			break
		}
	}
	// remove from the backing list, then recompute the filtered view.
	for i, s := range h.allSessions {
		if s.ID == msg.id {
			h.allSessions = append(h.allSessions[:i], h.allSessions[i+1:]...)
			break
		}
	}
	h.applyFilter()
	h.rebuildTable()
	if deletedIdx >= 0 && len(h.sessions) > 0 {
		cur := deletedIdx
		if cur >= len(h.sessions) {
			cur = len(h.sessions) - 1
		}
		h.table.SetCursor(cur)
	}
	return h, nil
}

func (h Agents) onAssignResult(msg assignModelResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// revert optimistic local update on failure.
		h.status = connErrStyle.Render("assign failed: " + msg.err.Error())
		h.setSessionModel(msg.id, "")
		return h, nil
	}
	// refresh running info and fetch caps for the newly assigned model.
	var cmds []tea.Cmd
	cmds = append(cmds, fetchRunning(h.client))
	for _, s := range h.allSessions {
		if s.ID == msg.id && s.Model != "" {
			if _, ok := h.modelCaps[s.Model]; !ok {
				cmds = append(cmds, fetchModelCapsCmd(h.client, s.Model))
			}
			break
		}
	}
	return h, tea.Batch(cmds...)
}

func (h Agents) onUnassignResult(msg unassignModelResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// revert optimistic local update on failure.
		h.status = connErrStyle.Render("unassign failed: " + msg.err.Error())
		return h, fetchSessions(h.client)
	}
	return h, nil
}

func (h Agents) onExportResult(msg exportChatResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		h.infoMsg = connErrStyle.Render("[error] " + msg.err.Error())
	} else {
		h.infoMsg = modelsStyle.Render("[info] exported → " + msg.path)
	}
	return h, nil
}

func (h Agents) onAssignModel(msg AssignModelMsg) (tea.Model, tea.Cmd) {
	// optimistically update the local session so the table reflects the change immediately.
	// assignModelCmd fires concurrently to persist the assignment in inarid.
	sessionName := msg.SessionID
	for _, s := range h.allSessions {
		if s.ID == msg.SessionID {
			sessionName = s.Name
			break
		}
	}
	h.setSessionModel(msg.SessionID, msg.ModelName)
	return h, assignModelCmd(h.client, msg.SessionID, sessionName, msg.ModelName)
}

func (h Agents) onUnassignModel(msg UnassignModelMsg) (tea.Model, tea.Cmd) {
	// optimistically clear the session's model so the table updates immediately.
	// unassignModelCmd fires concurrently to persist the change in inarid.
	sessionName, model := msg.SessionName, ""
	for _, s := range h.allSessions {
		if s.ID == msg.SessionID {
			model = s.Model
			sessionName = s.Name
			break
		}
	}
	h.setSessionModel(msg.SessionID, "")
	return h, unassignModelCmd(h.client, msg.SessionID, sessionName, model)
}
