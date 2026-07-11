package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func cmdStart(cfgPath string, verbose bool) {
	// refuse to start a second daemon; two daemons sharing the same socket causes
	// the TUI's shared Call connection and fresh ChatStream connections to hit
	// different processes with different in-memory session state.
	if pid, err := readPID(); err == nil {
		if proc, err := os.FindProcess(pid); err == nil && proc.Signal(syscall.Signal(0)) == nil {
			log.Fatalf("inari daemon already running (pid %d); run 'inari stop' first", pid)
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
