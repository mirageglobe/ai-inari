// fox is a CLI companion to kitsune (TUI) and inarid (daemon).
// it provides scriptable access to inari sessions over the Unix socket,
// letting you send prompts, inspect sessions, and check daemon health
// without opening the full terminal UI.
//
// usage:
//
//	fox ping                       check if inarid is running
//	fox sessions                   list all sessions
//	fox chat <session-id> <message> send a message to a session
//	fox help                       show usage
package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

const socketPath = "/tmp/inari.sock"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	switch args[0] {
	case "ping":
		cmdPing()
	case "sessions":
		cmdSessions()
	case "chat":
		if len(args) < 3 {
			fatalf("usage: fox chat <session-id> <message>")
		}
		cmdChat(args[1], strings.Join(args[2:], " "))
	case "help", "--help", "-h":
		printUsage()
	default:
		fatalf("unknown command %q — run `fox help` for usage", args[0])
	}
}

// dial connects to inarid and exits with a clear error if the daemon is not running.
func dial() *ipc.Client {
	c := ipc.NewClient(socketPath)
	if err := c.TryReconnect(); err != nil {
		fatalf("inarid is not running — start it with `make start`")
	}
	return c
}

func cmdPing() {
	c := dial()
	defer c.Close()
	if err := c.Ping(); err != nil {
		fatalf("ping failed: %v", err)
	}
	fmt.Println("pong")
}

func cmdSessions() {
	c := dial()
	defer c.Close()
	sessions, err := c.ListSessions()
	if err != nil {
		fatalf("list sessions: %v", err)
	}
	if len(sessions) == 0 {
		fmt.Println("no sessions")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "name\tid\tmodel")
	for _, s := range sessions {
		model := s.Model
		if model == "" {
			model = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.ID, model)
	}
	w.Flush()
}

func cmdChat(sessionID, msg string) {
	c := dial()
	defer c.Close()
	reply, err := c.Chat(sessionID, msg)
	if err != nil {
		fatalf("chat: %v", err)
	}
	fmt.Println(reply)
}

func printUsage() {
	fmt.Print(`fox — CLI access to your inari sessions

usage:
  fox ping                         check if inarid is running
  fox sessions                     list all sessions
  fox chat <session-id> <message>  send a message to a session
  fox help                         show this help

run ` + "`fox sessions`" + ` to find a session ID.
`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
