// Package main is the inarid daemon — the persistent background engine for ai-inari.
//
// Responsibilities:
//   - Verify Ollama is reachable before accepting any connections.
//   - Bind a Unix Domain Socket (chmod 0600) and serve JSON-RPC 2.0 requests.
//   - Own the session store: sessions are the primary entity. Each session has a
//     name (e.g. "Arctic Fox"), an optional model, and its full chat history.
//     Sessions survive fox detach/reattach; fox is stateless.
//   - Enforce the memory/concurrency budget via the scheduler semaphore.
//   - Spawn and manage MCP connector child processes (filesystem, search, SQL).
//   - Append-only audit log every tool-call with a timestamp.
//   - Shut down cleanly on SIGINT, SIGTERM, or a daemon.quit RPC from fox.
//
// inarid must be started before fox. It runs until explicitly stopped.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/config"
	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/mcp"
	"github.com/mirageglobe/ai-inari/internal/ollama"
	"github.com/mirageglobe/ai-inari/internal/scheduler"
	"github.com/mirageglobe/ai-inari/internal/session"
)

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	auditor := audit.New("inari-audit.log")
	defer auditor.Close()

	ollamaClient := ollama.NewClient(cfg.OllamaBaseURL)
	if err := ollamaClient.Ping(); err != nil {
		log.Printf("ERROR: ollama endpoint not reachable: %v", err)
		log.Printf("ERROR: expected ollama at %s — is it running? (e.g. `ollama serve`)", cfg.OllamaBaseURL)
		log.Fatal("ERROR: inarid cannot start without ollama")
	}
	log.Printf("ollama ready at %s", cfg.OllamaBaseURL)

	sched := scheduler.New(cfg.MemoryBudgetMB)
	store := session.NewStore()
	mcpHost := mcp.NewHost(cfg.MCPConnectors, auditor)

	if err := mcpHost.Start(); err != nil {
		log.Fatalf("mcp: %v", err)
	}
	defer mcpHost.Stop()

	srv, err := ipc.NewServer(cfg.Socket, store, sched, mcpHost, auditor, ollamaClient)
	if err != nil {
		log.Fatalf("ipc: %v", err)
	}
	defer srv.Close()

	log.Printf("inarid listening on %s (ctrl+c to quit)", cfg.Socket)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case <-srv.Quit():
	}

	log.Println("inarid shutting down")
}
