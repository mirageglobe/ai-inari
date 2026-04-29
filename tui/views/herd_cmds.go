// Package views — herd data plumbing: internal message types, tea.Cmd constructors,
// session naming, and shared formatting helpers.
// it does NOT own the Herd view struct or its rendering — those live in herd.go.
package views

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/provider"
)

// runningMeta holds live stats for a running model, used to populate VRAM/Status columns.
type runningMeta struct {
	vram   int64
	expiry string
}

type runningMsg struct {
	models []provider.RunningModel
	err    error
}

type sessionsMsg struct {
	sessions []ipc.SessionInfo
	err      error
}

type createSessionResultMsg struct {
	session ipc.SessionInfo
	err     error
}

type deleteSessionResultMsg struct {
	id  string
	err error
}

type assignModelResultMsg struct {
	id  string
	err error
}

type unassignModelResultMsg struct {
	id  string
	err error
}

type exportChatResultMsg struct {
	path string
	err  error
}

// kitsunAdjectives are paired with "kitsune" to form session names like "jade kitsune".
var kitsuneAdjectives = []string{
	"arctic", "amber", "ash", "blaze", "copper", "crimson", "dusk",
	"ember", "fire", "frost", "ghost", "golden", "jade", "midnight",
	"rusty", "scarlet", "shadow", "silver", "storm", "swift", "thunder",
	"tundra", "violet", "wild",
}

// pickFoxName returns a kitsune-themed name not already in use.
func pickFoxName(used []string) string {
	inUse := make(map[string]bool, len(used))
	for _, v := range used {
		inUse[v] = true
	}
	pool := make([]string, len(kitsuneAdjectives))
	copy(pool, kitsuneAdjectives)
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	for _, adj := range pool {
		name := adj + " kitsune"
		if !inUse[name] {
			return name
		}
	}
	return fmt.Sprintf("kitsune #%d", len(used)+1)
}

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

// formatBytes formats a byte count as a human-readable string (GB/MB/B).
func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(b)/float64(1<<20))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// formatExpiry formats an RFC3339 expiry timestamp as a human-readable countdown.
func formatExpiry(expiresAt string) string {
	t, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return "—"
	}
	d := time.Until(t).Round(time.Second)
	if d <= 0 {
		return "waking"
	}
	return fmt.Sprintf("in %s", d)
}
