package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mirageglobe/ai-haniwa/internal/ipc"
	"github.com/mirageglobe/ai-haniwa/tui"
)

const defaultSocket = "/tmp/haniwa.sock"

func main() {
	client, err := ipc.NewClient(defaultSocket)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer client.Close()

	p := tea.NewProgram(tui.New(client), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("tui: %v", err)
	}
}
