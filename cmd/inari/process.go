package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/mirageglobe/inari/internal/config"
)

// refuseIfRunning fatals if a live daemon is already recorded; two daemons sharing
// one socket makes the TUI's shared Call connection and fresh ChatStream connections
// hit different processes with different in-memory session state. a stale pid file
// from a crashed daemon is cleared so a fresh one can take its place.
func refuseIfRunning() {
	if pid, err := readPID(); err == nil {
		if alive(pid) {
			log.Fatalf("inari daemon already running (pid %d); run 'inari stop' first", pid)
		}
		os.Remove(pidFile())
	}
}

// forkDaemon spawns the detached daemon worker (`daemon --child`) and waits for its
// socket, returning the child pid. callers must refuseIfRunning first. it waits on
// the socket the child will actually bind (from cfgPath), not the compiled default,
// so a custom `socket` in config.json is honoured instead of a false "did not come up".
func forkDaemon(cfgPath string, verbose bool) (int, error) {
	socket := defaultSocket
	if cfg, err := config.Load(cfgPath); err == nil && cfg.Socket != "" {
		socket = cfg.Socket
	}
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	args := []string{"daemon", "--child", "-config", cfgPath}
	if verbose {
		args = append(args, "-v")
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	if !waitForSocket(socket, 5*time.Second) {
		return cmd.Process.Pid, fmt.Errorf("daemon did not come up within 5s")
	}
	return cmd.Process.Pid, nil
}

// cmdStart forks the background daemon and opens the TUI (bare `inari` / `inari start`).
func cmdStart(cfgPath string, verbose bool) {
	refuseIfRunning()
	pid, err := forkDaemon(cfgPath, verbose)
	if err != nil {
		log.Fatalf("start daemon: %v", err)
	}
	fmt.Printf("inari daemon started (pid %d)\n", pid)
	runTUI(cfgPath)
}

// cmdDaemon runs the daemon. by default it self-detaches into the background and
// returns, so `inari stop` manages it; -f/--foreground runs it attached (ctrl+c to
// quit). --child is the internal marker set by forkDaemon meaning "you are the
// forked worker, run the server loop now".
func cmdDaemon(cfgPath string, verbose, foreground, child bool) {
	if child {
		runDaemon(cfgPath, verbose, false) // detached worker: not attached, no ctrl+c line
		return
	}
	refuseIfRunning()
	if foreground {
		runDaemon(cfgPath, verbose, true) // attached: prints ctrl+c to quit
		return
	}
	pid, err := forkDaemon(cfgPath, verbose)
	if err != nil {
		log.Fatalf("start daemon: %v", err)
	}
	fmt.Printf("inari daemon started (pid %d)\n", pid)
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
	// wait for the process to actually exit before returning: the daemon's deferred
	// srv.Close() unlinks its socket file, so a caller that immediately starts a new
	// daemon at the same path risks the old process deleting the new one's socket.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if proc.Signal(syscall.Signal(0)) != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	os.Remove(pidFile())
	fmt.Printf("inari daemon stopped (pid %d)\n", pid)
}
