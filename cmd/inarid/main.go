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
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
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
	verbose := flag.Bool("v", false, "verbose logging: print every RPC call and response")
	flag.Parse()

	log.Println("awakening inari daemon 🦊")

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	auditor := audit.New("inari-audit.log")
	defer auditor.Close()

	ollamaClient := ollama.NewClient(cfg.OllamaBaseURL)
	if err := ollamaClient.Ping(); err != nil {
		log.Printf("ollama not reachable: %v", err)
		log.Printf("expected at: %s", cfg.OllamaBaseURL)
		log.Fatal("hint: run `ollama serve` then retry")
	}
	log.Printf("ollama ready: %s", cfg.OllamaBaseURL)

	sched := scheduler.New(cfg.MemoryBudgetMB)

	dataDir := cfg.DataDir
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("data dir: %v", err)
		}
		dataDir = filepath.Join(home, ".local", "share", "inari", "sessions")
	}
	store, err := session.NewPersistentStore(dataDir)
	if err != nil {
		log.Fatalf("session store: %v", err)
	}
	log.Printf("sessions: %s", dataDir)
	mcpHost := mcp.NewHost(cfg.MCPConnectors, auditor)

	if err := mcpHost.Start(); err != nil {
		log.Fatalf("mcp: %v", err)
	}
	defer mcpHost.Stop()

	srv, err := ipc.NewServer(cfg.Socket, store, sched, mcpHost, auditor, ollamaClient, *verbose)
	if err != nil {
		log.Fatalf("ipc: %v", err)
	}
	defer srv.Close()

	log.Printf("listening: %s", cfg.Socket)
	log.Println("ctrl+c to quit")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case <-srv.Quit():
	}

	log.Println("inarid shutting down")
}
