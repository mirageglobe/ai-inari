package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mirageglobe/inari/internal/audit"
	"github.com/mirageglobe/inari/internal/config"
	"github.com/mirageglobe/inari/internal/ipc"
	"github.com/mirageglobe/inari/internal/mcp"
	"github.com/mirageglobe/inari/internal/ollama"
	"github.com/mirageglobe/inari/internal/session"
)

// runDaemon is the foreground daemon loop shared by "daemon" and the background
// fork spawned by "start". attached is true only for `inari daemon -f` (foreground,
// prints ctrl+c); the detached worker and the default background path pass false.
func runDaemon(cfgPath string, verbose, attached bool) {
	if verbose {
		log.SetPrefix("[log] ")
	}
	log.Println("inari daemon starting")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// auto-approve allowlist for execute_shell_command; empty keeps the default.
	// set before NewServer starts accepting so the value is stable under serving.
	ipc.SetShellAllowlist(cfg.Shell.Allowlist)

	// resolve the active backend from a named endpoint profile, falling back to the
	// legacy single ollama_base_url when no provider is selected.
	endpoint, named := cfg.ActiveEndpoint()
	if cfg.Provider != "" && !named {
		log.Printf("provider %q not found in endpoints; falling back to %s", cfg.Provider, endpoint.BaseURL)
	}
	ollamaClient := ollama.NewClientWithAuth(endpoint.BaseURL, endpoint.APIKey, endpoint.Headers)
	ollamaClient.SetVerbose(verbose)
	ollamaClient.SetKeepAlive(cfg.Ollama.KeepAlive) // "" leaves Ollama's own default
	if err := ollamaClient.Ping(); err != nil {
		log.Printf("ollama not reachable: %v", err)
		log.Printf("expected at: %s", endpoint.BaseURL)
		log.Fatal("hint: run `ollama serve` then retry")
	}
	log.Printf("ollama ready: %s", endpoint.BaseURL)

	dataDir := resolveDataDir(cfg.DataDir)
	store, err := session.NewPersistentStore(dataDir)
	if err != nil {
		log.Fatalf("session store: %v", err)
	}
	log.Printf("sessions: %s", dataDir)

	// audit log lives beside the session store, not the daemon's cwd (which varies
	// by launch context and previously scattered "inari-audit.log" into whatever
	// directory started the daemon). doctor computes the same path via auditLogPath.
	auditPath := auditLogPath(cfg.DataDir)
	auditor := audit.New(auditPath)
	defer auditor.Close()
	log.Printf("audit log: %s", auditPath)

	mcpHost := mcp.NewHost(cfg.MCPConnectors, auditor)
	if err := mcpHost.Start(); err != nil {
		log.Fatalf("mcp: %v", err)
	}
	defer mcpHost.Stop()

	// idle auto-shutdown window; the 0->30min / negative->disabled logic lives in
	// config.IdleTimeout so the default is defined in one place.
	idleTimeout := cfg.IdleTimeout()
	if idleTimeout > 0 {
		log.Printf("idle auto-shutdown: %s", idleTimeout)
	}

	srv, err := ipc.NewServer(ipc.ServerConfig{
		Socket:             cfg.Socket,
		Store:              store,
		Auditor:            auditor,
		Provider:           ollamaClient,
		Verbose:            verbose,
		IdleTimeout:        idleTimeout,
		DefaultModel:       cfg.Models.Thinker,
		GlobalSystemPrompt: cfg.Context.SystemPrompt,
	})
	if err != nil {
		log.Fatalf("ipc: %v", err)
	}
	defer srv.Close()

	// always record the pid so `inari stop` can find this daemon regardless of how
	// it was launched (attached or detached); a foreground daemon used to skip this
	// and so could not be stopped via the pid file.
	if err := writePID(os.Getpid()); err != nil {
		log.Printf("pid file warning: %v", err)
	}

	log.Printf("listening: %s", cfg.Socket)
	if attached {
		log.Println("ctrl+c to quit")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case <-srv.Quit():
	}

	os.Remove(pidFile())
	log.Println("inari daemon shutting down")
}
