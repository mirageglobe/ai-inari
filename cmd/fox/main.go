// Package main is fox — the terminal UI client for ai-inari.
//
// Responsibilities:
//   - Connect to a running inarid over its Unix Domain Socket.
//   - Render the keyboard-driven TUI (herd view, logs, describe, chat).
//   - Forward user input to inarid as JSON-RPC calls and display responses.
//   - Detach cleanly on quit; inarid and all sessions keep running.
//
// fox is stateless: all session state lives in inarid. Restarting fox
// reconnects to the existing herd without interrupting any running models.
package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/tui"
)

const defaultSocket = "/tmp/inari.sock"

func main() {
	client := ipc.NewClient(defaultSocket)

	// Prevent lipgloss from querying the terminal background colour via OSC 11;
	// without this, the terminal's response leaks into the textarea as raw text.
	lipgloss.SetHasDarkBackground(true)

	p := tea.NewProgram(tui.New(client), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("tui: %v", err)
	}

	client.Close()
}
