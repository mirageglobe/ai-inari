package main

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/inari/internal/config"
	"github.com/mirageglobe/inari/internal/ipc"
	"github.com/mirageglobe/inari/tui"
	"github.com/mirageglobe/inari/tui/views"
)

func runTUI(cfgPath string) {
	if f, err := os.OpenFile("inari.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		log.SetOutput(f)
		defer f.Close()
	}

	themeIdx := 0
	if cfg, err := config.Load(cfgPath); err == nil && cfg.Theme != "" {
		themeIdx = views.ThemeIndex(cfg.Theme)
	}
	views.ApplyTheme(views.Themes[themeIdx])

	// prevent lipgloss from querying the terminal background colour via OSC 11;
	// without this, the terminal's response leaks into the textarea as raw text.
	lipgloss.SetHasDarkBackground(true)

	client := ipc.NewClient(defaultSocket)
	p := tea.NewProgram(tui.New(client, cfgPath, themeIdx), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		log.Fatalf("tui: %v", err)
	}
	// restore terminal cursor shape; the TUI emits DECSCUSR sequences and Bubble Tea
	// does not reset the cursor on alt-screen exit, so bash inherits the last shape set.
	fmt.Print(views.ResetCursor)
	client.Close()
}
