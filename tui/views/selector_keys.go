// selector_keys.go owns ModelSelector's key handling: the disk-delete confirm
// and the u/d/enter/l actions. it does NOT own the Update dispatch
// (selector_update.go), the struct/construction (selector.go), or rendering
// (selector_render.go).

package views

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/inari/internal/provider"
)

// handleKey processes a key press for the model selector. handled reports
// whether the key was consumed; when false the caller falls through to the
// table's own navigation update, preserving the prior behaviour.
func (m ModelSelector) handleKey(msg tea.KeyMsg) (ModelSelector, tea.Cmd, bool) {
	// a disk delete is armed: [y] confirms, any other key cancels in place.
	// esc/q never reach here (the root closes the modal first), which also cancels.
	if m.pendingDelete != "" {
		name := m.pendingDelete
		m.pendingDelete = ""
		if msg.String() == "y" {
			m.loading = true
			m.status = modelsStyle.Render("deleting " + name + "...")
			return m, tea.Batch(
				m.spinner.Tick,
				func() tea.Msg { return deleteModelMsg{name: name, err: m.client.DeleteModel(name)} },
			), true
		}
		m.status = "" // cancelled
		return m, nil, true
	}
	switch msg.String() {
	case "u":
		// unload (unassign) the session's current model; no-op if none assigned.
		if !m.loading && m.targetModel != "" {
			id, name := m.targetSessionID, m.targetSessionName
			return m, func() tea.Msg { return UnassignModelMsg{SessionID: id, SessionName: name} }, true
		}
	case "d":
		// delete a downloaded model from disk; arms a confirm ([pull] rows are a no-op).
		// destructive + irreversible, so require [y] before the delete fires (§8.2).
		if !m.loading {
			idx := m.table.Cursor()
			if idx >= 0 && idx < len(m.rowLocal) && m.rowLocal[idx] {
				name := m.rowModel[idx]
				m.pendingDelete = name
				warn := ""
				if name == m.targetModel {
					warn = " (assigned to " + m.targetSessionName + ")"
				}
				m.status = connErrStyle.Render("delete " + name + warn + " from disk? [y] confirm  [n] cancel")
			}
		}
		// consume [d] whether or not it armed, so it never doubles as a table nav key.
		return m, nil, true
	case "enter", "l":
		if !m.loading {
			idx := m.table.Cursor()
			if row := m.table.SelectedRow(); len(row) > 0 && idx >= 0 && idx < len(m.rowLocal) {
				name, size := m.rowModel[idx], row[1]
				m.loading = true
				if m.rowLocal[idx] {
					m.status = modelsStyle.Render("loading " + name + " (" + size + ") → " + m.targetSessionName + "...")
					return m, tea.Batch(
						m.spinner.Tick,
						func() tea.Msg {
							return loadModelMsg{name: name, err: m.client.LoadModel(name)}
						},
					), true
				}

				progress := make(chan provider.PullProgress, 8)
				errc := make(chan error, 1)
				go func() {
					err := m.client.PullModel(name, progress)
					errc <- err
					close(progress)
				}()
				m.pullProgress, m.pullErrc, m.pullTarget = progress, errc, name
				m.status = modelsStyle.Render("pulling " + name + "...")
				return m, tea.Batch(m.spinner.Tick, readNextPullUpdate(name, progress, errc)), true
			}
		}
	}
	return m, nil, false
}
