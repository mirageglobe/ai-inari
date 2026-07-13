// selector_update.go owns ModelSelector's Update method. it does NOT own the
// struct/construction (selector.go) or rendering (selector_render.go).

package views

import (
	"sort"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

func (m ModelSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ThemeChangedMsg:
		ApplyTableStyles(&m.table)
		m.spinner.Style = spinnerStyle
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		// topbar(1) + models header(1) + border-top(1) + col-header(1) + border-bottom(1) + status(1) + hint(1) = 7 reserved
		tableHeight := msg.Height - 6
		if tableHeight < 1 {
			tableHeight = 1
		}
		m.table.SetHeight(tableHeight)
		// recompute columns for the new width and re-truncate notes to fit.
		m.refreshRows()
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case modelsMsg:
		if msg.err == nil {
			sort.Slice(msg.models, func(i, j int) bool {
				return msg.models[i].Name < msg.models[j].Name
			})
			names := make([]string, len(msg.models))
			for i, model := range msg.models {
				names[i] = model.Name
			}
			m.localModels = msg.models
			m.recommended = NotLocal(names)
			m.refreshRows()
		}
		return m, nil

	case runningMsg:
		if msg.err == nil {
			set := make(map[string]bool, len(msg.models))
			for _, rm := range msg.models {
				set[rm.Name] = true
			}
			m.running = set
			m.refreshRows()
		}
		return m, nil

	case loadModelMsg:
		m.loading = false
		if msg.err != nil {
			m.status = connErrStyle.Render("load failed: " + msg.err.Error())
			return m, nil
		}
		if m.targetSessionID == "" {
			return m, func() tea.Msg { return BackToAgentsMsg{} }
		}
		id, name := m.targetSessionID, msg.name
		return m, func() tea.Msg { return AssignModelMsg{SessionID: id, ModelName: name} }

	case deleteModelMsg:
		m.loading = false
		if msg.err != nil {
			m.status = connErrStyle.Render("delete failed: " + msg.err.Error())
			return m, nil
		}
		m.status = modelsStyle.Render("deleted " + msg.name)
		// refresh so the freed model's row flips back to [pull] and drops from running.
		return m, tea.Batch(fetchModels(m.client), fetchRunning(m.client))

	case pullProgressMsg:
		if msg.model != m.pullTarget {
			return m, nil
		}
		m.status = modelsStyle.Render(pullStatusText(msg.model, msg.progress))
		return m, readNextPullUpdate(m.pullTarget, m.pullProgress, m.pullErrc)

	case pullDoneMsg:
		m.pullProgress, m.pullErrc, m.pullTarget = nil, nil, ""
		if msg.err != nil {
			m.loading = false
			m.status = connErrStyle.Render("pull failed: " + msg.err.Error())
			return m, nil
		}
		// pulled successfully; refresh the list and continue into the normal
		// load+assign flow, same as picking an already-available model.
		name := msg.model
		m.status = modelsStyle.Render("loading " + name + " → " + m.targetSessionName + "...")
		return m, tea.Batch(
			fetchModels(m.client),
			func() tea.Msg { return loadModelMsg{name: name, err: m.client.LoadModel(name)} },
		)

	case tea.KeyMsg:
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
				)
			}
			m.status = "" // cancelled
			return m, nil
		}
		switch msg.String() {
		case "u":
			// unload (unassign) the session's current model; no-op if none assigned.
			if !m.loading && m.targetModel != "" {
				id, name := m.targetSessionID, m.targetSessionName
				return m, func() tea.Msg { return UnassignModelMsg{SessionID: id, SessionName: name} }
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
			return m, nil
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
						)
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
					return m, tea.Batch(m.spinner.Tick, readNextPullUpdate(name, progress, errc))
				}
			}
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}
