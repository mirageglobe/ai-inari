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
		switch msg.String() {
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
