// Package main is kitsune — the terminal UI client for ai-inari.
//
// Responsibilities:
//   - Connect to a running inarid over its Unix Domain Socket.
//   - Render the keyboard-driven TUI (herd view, logs, describe, chat).
//   - Forward user input to inarid as JSON-RPC calls and display responses.
//   - Detach cleanly on quit; inarid and all sessions keep running.
//
// kitsune is stateless: all session state lives in inarid. Restarting kitsune
// reconnects to the existing herd without interrupting any running models.
package main

import (
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/config"
	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/tui"
	"github.com/mirageglobe/ai-inari/tui/views"
)

const (
	defaultSocket     = "/tmp/inari.sock"
	defaultConfigPath = "config.json"
)

func main() {
	// redirect log output to kitsune.log so IPC errors don't bleed into the TUI.
	if f, err := os.OpenFile("kitsune.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		log.SetOutput(f)
		defer f.Close()
	}

	// apply saved theme before the first render; fall back to default on missing/unknown.
	themeIdx := 0
	if cfg, err := config.Load(defaultConfigPath); err == nil && cfg.Theme != "" {
		themeIdx = views.ThemeIndex(cfg.Theme)
	}
	views.ApplyTheme(views.Themes[themeIdx])

	client := ipc.NewClient(defaultSocket)

	// prevent lipgloss from querying the terminal background colour via OSC 11;
	// without this, the terminal's response leaks into the textarea as raw text.
	lipgloss.SetHasDarkBackground(true)

	p := tea.NewProgram(tui.New(client, defaultConfigPath, themeIdx), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("tui: %v", err)
	}

	client.Close()
}
