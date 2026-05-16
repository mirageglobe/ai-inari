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
// Usage:
//
//	inarid start   — fork to background, write PID file
//	inarid stop    — send SIGTERM to the running daemon
//	inarid status  — report whether the daemon is running
//	inarid         — run in foreground (default, for debugging)
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/config"
	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/mcp"
	"github.com/mirageglobe/ai-inari/internal/ollama"
	"github.com/mirageglobe/ai-inari/internal/scheduler"
	"github.com/mirageglobe/ai-inari/internal/session"
)

func inariDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("home dir: %v", err)
	}
	return filepath.Join(home, ".local", "share", "inari")
}

func pidFile() string {
	return filepath.Join(inariDir(), "inarid.pid")
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("home dir: %v", err)
	}
	return filepath.Join(home, ".config", "inari", "config.json")
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidFile())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func writePID(pid int) error {
	path := pidFile()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
}

func cmdStart() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("executable: %v", err)
	}
	cmd := exec.Command(exe, "--daemon")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		log.Fatalf("start: %v", err)
	}
	if err := writePID(cmd.Process.Pid); err != nil {
		log.Fatalf("pid file: %v", err)
	}
	fmt.Printf("inarid started (pid %d)\n", cmd.Process.Pid)
}

func cmdStop() {
	pid, err := readPID()
	if err != nil {
		fmt.Println("inarid is not running (no pid file)")
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil || proc.Signal(syscall.Signal(0)) != nil {
		fmt.Println("inarid is not running")
		os.Remove(pidFile())
		return
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		log.Fatalf("stop: %v", err)
	}
	os.Remove(pidFile())
	fmt.Printf("inarid stopped (pid %d)\n", pid)
}

func cmdStatus() {
	pid, err := readPID()
	if err != nil {
		fmt.Println("inarid: not running")
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil || proc.Signal(syscall.Signal(0)) != nil {
		fmt.Printf("inarid: not running (stale pid %d)\n", pid)
		os.Remove(pidFile())
		return
	}
	fmt.Printf("inarid: running (pid %d)\n", pid)
}

func main() {
	verbose := flag.Bool("v", false, "verbose logging: print every RPC call and response")
	daemon := flag.Bool("daemon", false, "run as background daemon (used internally by 'start')")
	configFlag := flag.String("config", "", "path to config.json (default: ~/.config/inari/config.json)")
	flag.Parse()

	switch flag.Arg(0) {
	case "start":
		cmdStart()
		return
	case "stop":
		cmdStop()
		return
	case "status":
		cmdStatus()
		return
	}

	if *verbose {
		log.SetPrefix("[ctrl-c to quit][log] ")
	}

	log.Println("awakening inari daemon")

	cfgPath := defaultConfigPath()
	if *configFlag != "" {
		cfgPath = *configFlag
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	auditor := audit.New("inari-audit.log")
	defer auditor.Close()

	ollamaClient := ollama.NewClient(cfg.OllamaBaseURL)
	ollamaClient.SetVerbose(*verbose)
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

	if *daemon {
		if err := writePID(os.Getpid()); err != nil {
			log.Printf("pid file warning: %v", err)
		}
	}

	log.Printf("listening: %s", cfg.Socket)
	if !*daemon {
		log.Println("ctrl+c to quit")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case <-srv.Quit():
	}

	if *daemon {
		os.Remove(pidFile())
	}

	log.Println("inarid shutting down")
}
