package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mirageglobe/ai-sudama/internal/audit"
	"github.com/mirageglobe/ai-sudama/internal/config"
	"github.com/mirageglobe/ai-sudama/internal/ipc"
	"github.com/mirageglobe/ai-sudama/internal/mcp"
	"github.com/mirageglobe/ai-sudama/internal/scheduler"
	"github.com/mirageglobe/ai-sudama/internal/session"
)

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	auditor := audit.New("sudama-audit.log")
	defer auditor.Close()

	sched := scheduler.New(cfg.MemoryBudgetMB)
	store := session.NewStore()
	mcpHost := mcp.NewHost(cfg.MCPConnectors, auditor)

	if err := mcpHost.Start(); err != nil {
		log.Fatalf("mcp: %v", err)
	}
	defer mcpHost.Stop()

	srv, err := ipc.NewServer(cfg.Socket, store, sched, mcpHost, auditor)
	if err != nil {
		log.Fatalf("ipc: %v", err)
	}
	defer srv.Close()

	log.Printf("sudamad listening on %s", cfg.Socket)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("sudamad shutting down")
}
