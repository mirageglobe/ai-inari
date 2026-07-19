// describe_update.go owns the Describe view's Update method. it does NOT own
// the struct/construction (describe.go) or rendering (describe_render.go).

package views

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func (d Describe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ThemeChangedMsg:
		if d.ready {
			d.viewport.SetContent(d.buildContent())
		}
		return d, nil

	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		// topbar(1) + border-top(1) + border-bottom(1) + hint(1) = 4 reserved
		vpHeight := msg.Height - 4
		if vpHeight < 1 {
			vpHeight = 1
		}
		// subtract 2 for sessionsStyle NormalBorder so total width = UIWidth.
		if !d.ready {
			d.viewport = viewport.New(d.width-2, vpHeight)
			d.ready = true
		} else {
			d.viewport.Width = d.width - 2
			d.viewport.Height = vpHeight
		}
		d.viewport.SetContent(d.buildContent())
		d.input.SetWidth(max(d.width-2, 20))
		d.input.SetHeight(max(msg.Height-5, 3))
		return d, nil

	case describeHistoryMsg:
		if msg.err == nil {
			d.msgCount = msg.count
		}
		d.fetched = true
		if d.ready {
			d.viewport.SetContent(d.buildContent())
		}
		return d, nil

	case describeSetContextMsg:
		d.saving = false
		if msg.err != nil {
			d.saveErr = "save failed: " + msg.err.Error()
			return d, nil
		}
		d.systemPrompt = msg.prompt
		d.editing = false
		d.saveErr = ""
		if d.ready {
			d.viewport.SetContent(d.buildContent())
		}
		return d, nil

	case tea.KeyMsg:
		if d.editing {
			switch msg.String() {
			case "ctrl+s":
				if d.saving || d.offline {
					return d, nil
				}
				d.saving = true
				d.saveErr = ""
				return d, saveContext(d.client, d.sessID, d.input.Value())
			case "esc":
				d.editing = false
				d.saveErr = ""
				return d, nil
			}
			var cmd tea.Cmd
			d.input, cmd = d.input.Update(msg)
			return d, cmd
		}
		if msg.String() == "e" && d.sessID != "" && !d.offline {
			d.input = newContextInput(d.systemPrompt, d.width, d.height)
			d.editing = true
			d.saveErr = ""
			return d, d.input.Focus()
		}
	}

	if d.ready && !d.editing {
		var cmd tea.Cmd
		d.viewport, cmd = d.viewport.Update(msg)
		return d, cmd
	}
	return d, nil
}
