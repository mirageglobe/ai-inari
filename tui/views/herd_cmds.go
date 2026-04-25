// Package views — herd data plumbing: internal message types, tea.Cmd constructors,
// session naming, and shared formatting helpers.
// it does NOT own the Herd view struct or its rendering — those live in herd.go.
package views

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/ollama"
)

// runningMeta holds live stats for a running model, used to populate VRAM/Status columns.
type runningMeta struct {
	vram   int64
	expiry string
}

type runningMsg struct {
	models []ollama.RunningModel
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

// foxAdjectives are paired with "Fox" to form session names like "Arctic Fox".
var foxAdjectives = []string{
	"Arctic", "Amber", "Ash", "Blaze", "Copper", "Crimson", "Dusk",
	"Ember", "Fire", "Frost", "Ghost", "Golden", "Jade", "Midnight",
	"Rusty", "Scarlet", "Shadow", "Silver", "Storm", "Swift", "Thunder",
	"Tundra", "Violet", "Wild",
}

// pickFoxName returns a fox-themed name not already in use.
func pickFoxName(used []string) string {
	inUse := make(map[string]bool, len(used))
	for _, v := range used {
		inUse[v] = true
	}
	pool := make([]string, len(foxAdjectives))
	copy(pool, foxAdjectives)
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	for _, adj := range pool {
		name := adj + " Fox"
		if !inUse[name] {
			return name
		}
	}
	return fmt.Sprintf("Fox #%d", len(used)+1)
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
			log.Printf("kitsune %q (%s): model unloaded ← %s", sessionName, sessionID, model)
		}
		return unassignModelResultMsg{id: sessionID, err: err}
	}
}

func assignModelCmd(client *ipc.Client, sessionID, sessionName, model string) tea.Cmd {
	return func() tea.Msg {
		err := client.AssignModel(sessionID, model)
		if err == nil {
			log.Printf("kitsune %q (%s): model assigned → %s", sessionName, sessionID, model)
		}
		return assignModelResultMsg{id: sessionID, err: err}
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
