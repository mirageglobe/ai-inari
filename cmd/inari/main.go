// Package main is the inari unified entry point.
//
// Responsibilities:
//   - Parse the subcommand and dispatch to the correct mode.
//   - start:   fork daemon in background then launch TUI.
//   - daemon:  run the IPC server in the foreground (--background for internal use).
//   - tui:     run the terminal UI only (assumes daemon is already running).
//   - stop:    send SIGTERM to the running daemon.
//   - status:  report whether the daemon is running.
//   - version: print version string and exit.
//   - (no args): print help menu.
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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mirageglobe/ai-inari/internal/audit"
	"github.com/mirageglobe/ai-inari/internal/config"
	"github.com/mirageglobe/ai-inari/internal/ipc"
	"github.com/mirageglobe/ai-inari/internal/mcp"
	"github.com/mirageglobe/ai-inari/internal/ollama"
	"github.com/mirageglobe/ai-inari/internal/scheduler"
	"github.com/mirageglobe/ai-inari/internal/session"
	"github.com/mirageglobe/ai-inari/internal/version"
	"github.com/mirageglobe/ai-inari/tui"
	"github.com/mirageglobe/ai-inari/tui/views"
)

const defaultSocket = "/tmp/inari.sock"

func inariDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("home dir: %v", err)
	}
	return filepath.Join(home, ".local", "share", "inari")
}

func pidFile() string { return filepath.Join(inariDir(), "inari.pid") }

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

func printHelp() {
	fmt.Printf("inari %s\n", version.Version)
	fmt.Println()
	fmt.Println("usage:")
	fmt.Println("  inari <command> [flags]")
	fmt.Println()
	fmt.Println("commands:")
	fmt.Println("  start    launch daemon and open the TUI")
	fmt.Println("  tui      open TUI  (assumes daemon is running)")
	fmt.Println("  daemon   run daemon in foreground")
	fmt.Println("  stop     stop the running daemon")
	fmt.Println("  status   show daemon status")
	fmt.Println("  version  print version and exit")
	fmt.Println()
	fmt.Println("flags (follow the subcommand):")
	fmt.Println("  -v         verbose daemon logging")
	fmt.Println("  -config    path to config.json  (default: ~/.config/inari/config.json)")
}

// runDaemon is the foreground daemon loop shared by "daemon" and the background
// fork spawned by "start". background is true when forked internally.
func runDaemon(cfgPath string, verbose, background bool) {
	if verbose {
		log.SetPrefix("[log] ")
	}
	log.Println("inari daemon starting")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	auditor := audit.New("inari-audit.log")
	defer auditor.Close()

	ollamaClient := ollama.NewClient(cfg.OllamaBaseURL)
	ollamaClient.SetVerbose(verbose)
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

	srv, err := ipc.NewServer(cfg.Socket, store, sched, mcpHost, auditor, ollamaClient, verbose)
	if err != nil {
		log.Fatalf("ipc: %v", err)
	}
	defer srv.Close()

	if background {
		if err := writePID(os.Getpid()); err != nil {
			log.Printf("pid file warning: %v", err)
		}
	}

	log.Printf("listening: %s", cfg.Socket)
	if !background {
		log.Println("ctrl+c to quit")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case <-srv.Quit():
	}

	if background {
		os.Remove(pidFile())
	}
	log.Println("inari daemon shutting down")
}

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

func cmdStart(cfgPath string, verbose bool) {
	// refuse to start a second daemon; two daemons sharing the same socket causes
	// the TUI's shared Call connection and fresh ChatStream connections to hit
	// different processes with different in-memory session state.
	if pid, err := readPID(); err == nil {
		if proc, err := os.FindProcess(pid); err == nil && proc.Signal(syscall.Signal(0)) == nil {
			log.Fatalf("inari daemon already running (pid %d) — run 'inari stop' first", pid)
		}
		// stale pid file from a previous crash; remove it so the fresh daemon can write its own.
		os.Remove(pidFile())
	}

	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("executable: %v", err)
	}
	args := []string{"daemon", "--background", "-config", cfgPath}
	if verbose {
		args = append(args, "-v")
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		log.Fatalf("start daemon: %v", err)
	}
	fmt.Printf("inari daemon started (pid %d)\n", cmd.Process.Pid)
	runTUI(cfgPath)
}

func cmdStop() {
	pid, err := readPID()
	if err != nil {
		fmt.Println("inari daemon is not running (no pid file)")
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil || proc.Signal(syscall.Signal(0)) != nil {
		fmt.Println("inari daemon is not running")
		os.Remove(pidFile())
		return
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		log.Fatalf("stop: %v", err)
	}
	os.Remove(pidFile())
	fmt.Printf("inari daemon stopped (pid %d)\n", pid)
}

func cmdStatus() {
	pid, err := readPID()
	if err != nil {
		fmt.Println("inari daemon: not running")
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil || proc.Signal(syscall.Signal(0)) != nil {
		fmt.Printf("inari daemon: not running (stale pid %d)\n", pid)
		os.Remove(pidFile())
		return
	}
	fmt.Printf("inari daemon: running (pid %d)\n", pid)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	sub := os.Args[1]
	rest := os.Args[2:]

	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	verbose := fs.Bool("v", false, "verbose logging")
	background := fs.Bool("background", false, "run as background daemon (internal use)")
	cfgFlag := fs.String("config", "", "path to config.json")
	fs.Parse(rest) //nolint:errcheck

	cfgPath := defaultConfigPath()
	if *cfgFlag != "" {
		cfgPath = *cfgFlag
	}

	switch sub {
	case "start":
		cmdStart(cfgPath, *verbose)
	case "daemon":
		runDaemon(cfgPath, *verbose, *background)
	case "tui":
		runTUI(cfgPath)
	case "stop":
		cmdStop()
	case "status":
		cmdStatus()
	case "version":
		fmt.Println(version.Version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", sub)
		printHelp()
		os.Exit(1)
	}
}
