package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

// runChat is the headless entry point: it drives a single non-streaming turn
// against an existing session and prints the assistant reply, with no TUI.
// session.chat is a plain provider round-trip (no tool-call loop), so the path
// is deterministic and scriptable for automation and testing.
func runChat(args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	session := fs.String("session", "", "session id to send the message to (required)")
	message := fs.String("message", "", "message text; use - to read from stdin")
	asJSON := fs.Bool("json", false, "print the reply as a JSON object")
	cfgFlag := fs.String("config", "", "path to config.json")
	fs.Parse(args) //nolint:errcheck

	if strings.TrimSpace(*session) == "" {
		fmt.Fprintln(os.Stderr, "error: --session is required")
		os.Exit(2)
	}
	msg, err := resolveMessage(*message, os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	cfgPath := defaultConfigPath()
	if *cfgFlag != "" {
		cfgPath = *cfgFlag
	}

	// headless callers should not have to start the daemon by hand.
	ensureDaemon(cfgPath)

	client := ipc.NewClient(defaultSocket)
	defer client.Close()

	reply, err := client.Chat(*session, msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printReply(os.Stdout, reply, *asJSON)
}

// resolveMessage returns the message body from the --message flag, reading stdin
// when the flag is "-". an empty message (flag or stdin) is an error.
func resolveMessage(flagVal string, stdin io.Reader) (string, error) {
	if flagVal == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		msg := strings.TrimRight(string(b), "\n")
		if strings.TrimSpace(msg) == "" {
			return "", errors.New("empty message on stdin")
		}
		return msg, nil
	}
	if strings.TrimSpace(flagVal) == "" {
		return "", errors.New("--message is required (or pass - to read stdin)")
	}
	return flagVal, nil
}

// printReply writes the assistant reply as plain text, or a JSON object under asJSON.
func printReply(w io.Writer, reply string, asJSON bool) {
	if asJSON {
		b, _ := json.Marshal(map[string]string{"reply": reply})
		fmt.Fprintln(w, string(b))
		return
	}
	fmt.Fprintln(w, reply)
}

// ensureDaemon starts a background daemon and waits for its socket when none is
// already running; a live daemon is reused as-is.
func ensureDaemon(cfgPath string) {
	if daemonRunning() {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("executable: %v", err)
	}
	cmd := exec.Command(exe, "daemon", "--background", "-config", cfgPath)
	if err := cmd.Start(); err != nil {
		log.Fatalf("start daemon: %v", err)
	}
	if !waitForSocket(defaultSocket, 5*time.Second) {
		log.Fatalf("daemon did not come up within 5s")
	}
}

// daemonRunning reports whether a live daemon is recorded in the pid file,
// clearing a stale pid file (process gone) as a side effect.
func daemonRunning() bool {
	pid, err := readPID()
	if err != nil {
		return false
	}
	if proc, err := os.FindProcess(pid); err == nil && proc.Signal(syscall.Signal(0)) == nil {
		return true
	}
	os.Remove(pidFile()) // stale pid file from a previous crash
	return false
}

// waitForSocket polls the UDS until it accepts a connection or the timeout elapses.
func waitForSocket(socket string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if conn, err := net.Dial("unix", socket); err == nil {
			conn.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
