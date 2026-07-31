// sessions_cmds.go owns the Sessions view's tea.Cmd constructors: RPC calls for
// listing sessions/running models, create/delete/assign/unassign, chat
// export, and model-caps lookup. it does NOT own message types
// (sessions_msgs.go) or naming/formatting helpers (sessions_fmt.go).

package views

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/inari/internal/ipc"
)

func fetchSessions(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		sessions, err := client.ListSessions()
		if err != nil {
			return sessionsMsg{err: err}
		}
		return sessionsMsg{sessions: sessions}
	}
}

func fetchRunning(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		models, err := client.ListRunning()
		if err != nil {
			return runningMsg{err: err}
		}
		return runningMsg{models: models}
	}
}

func createSessionCmd(client *ipc.Client, name string) tea.Cmd {
	return func() tea.Msg {
		cwd, _ := os.Getwd()
		sess, err := client.CreateSession(name, cwd)
		return createSessionResultMsg{session: sess, err: err}
	}
}

func deleteSessionCmd(client *ipc.Client, id string) tea.Cmd {
	return func() tea.Msg {
		err := client.DeleteSession(id)
		return deleteSessionResultMsg{id: id, err: err}
	}
}

func unassignModelCmd(client *ipc.Client, sessionID, sessionName, model string) tea.Cmd {
	return func() tea.Msg {
		err := client.UnassignModel(sessionID)
		if err == nil {
			log.Printf("inariui %q (%s): model unloaded ← %s", sessionName, sessionID, model)
		}
		return unassignModelResultMsg{id: sessionID, err: err}
	}
}

func assignModelCmd(client *ipc.Client, sessionID, sessionName, model string) tea.Cmd {
	return func() tea.Msg {
		err := client.AssignModel(sessionID, model)
		if err == nil {
			log.Printf("inariui %q (%s): model assigned → %s", sessionName, sessionID, model)
		}
		return assignModelResultMsg{id: sessionID, err: err}
	}
}

func exportChatCmd(client *ipc.Client, sessionID, sessionName string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := client.History(sessionID)
		if err != nil {
			return exportChatResultMsg{err: err}
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return exportChatResultMsg{err: err}
		}
		dir := filepath.Join(home, ".local", "share", "inari", "exports")
		if err := os.MkdirAll(dir, 0750); err != nil {
			return exportChatResultMsg{err: err}
		}
		stamp := time.Now().Format("20060102-150405")
		safeName := strings.ReplaceAll(sessionName, " ", "-")
		path := filepath.Join(dir, safeName+"-"+stamp+".txt")

		var b strings.Builder
		for i, m := range msgs {
			if i > 0 {
				b.WriteString("---\n")
			}
			b.WriteString(m.Role + ": " + m.Content + "\n")
		}
		if err := os.WriteFile(path, []byte(b.String()), 0640); err != nil {
			return exportChatResultMsg{err: err}
		}
		log.Printf("inariui %q (%s): exported to %s", sessionName, sessionID, path)
		return exportChatResultMsg{path: path}
	}
}

func fetchModelCapsCmd(client *ipc.Client, model string) tea.Cmd {
	return func() tea.Msg {
		caps, _ := client.ModelCaps(model)
		return modelCapsMsg{model: model, caps: caps}
	}
}
