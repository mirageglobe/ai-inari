package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mirageglobe/ai-inari/internal/version"
)

const defaultSocket = "/tmp/inari.sock"

func printHelp() {
	fmt.Printf("inari %s\n", version.Version)
	fmt.Println()
	fmt.Println("usage:")
	fmt.Println("  inari [command] [flags]")
	fmt.Println()
	fmt.Println("commands:")
	fmt.Println("  (none)   launch daemon and open the TUI  (default)")
	fmt.Println("  tui      open TUI  (assumes daemon is running)")
	fmt.Println("  chat     send one message to a session, print the reply  (headless)")
	fmt.Println("  daemon   run daemon in foreground")
	fmt.Println("  doctor   check dependencies and daemon status (--models to run each model)")
	fmt.Println("  stop     stop the running daemon")
	fmt.Println("  version  print version and exit")
	fmt.Println("  help     show this message")
	fmt.Println()
	fmt.Println("flags (follow the subcommand):")
	fmt.Println("  -v         verbose daemon logging")
	fmt.Println("  -config    path to config.json  (default: ~/.config/inari/config.json)")
	fmt.Println("  -models    (doctor) run each configured model through a real tool-calling turn")
	fmt.Println()
	fmt.Println("chat flags:")
	fmt.Println("  -session   existing session id to send to   (or -new)")
	fmt.Println("  -new       create a new session for this turn")
	fmt.Println("  -name      name for the -new session         (default: generated)")
	fmt.Println("  -model     model for the -new session        (default: daemon default)")
	fmt.Println("  -cwd       working dir for the -new session  (default: none)")
	fmt.Println("  -message   message text, or - to read from stdin")
	fmt.Println("  -json      print the reply as a JSON object")
}

func main() {
	// bare invocation is the default user path: fork the daemon and open the TUI.
	if len(os.Args) < 2 {
		cmdStart(defaultConfigPath(), false)
		return
	}

	sub := os.Args[1]
	rest := os.Args[2:]

	// help and version need no flag parsing and no config resolution; chat parses
	// its own flags (--session/--message/--json) so it bypasses the shared parser.
	switch sub {
	case "help", "-h", "--help":
		printHelp()
		return
	case "version", "--version":
		fmt.Println(version.Version)
		return
	case "chat":
		runChat(rest)
		return
	}

	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	verbose := fs.Bool("v", false, "verbose logging")
	background := fs.Bool("background", false, "run as background daemon (internal use)")
	verifyModels := fs.Bool("models", false, "doctor: also run each configured model through a real tool-calling turn")
	cfgFlag := fs.String("config", "", "path to config.json")
	fs.Parse(rest) //nolint:errcheck

	cfgPath := defaultConfigPath()
	if *cfgFlag != "" {
		cfgPath = *cfgFlag
	}

	switch sub {
	case "start": // alias of the bare invocation; keeps `make start` and muscle memory working
		cmdStart(cfgPath, *verbose)
	case "daemon":
		runDaemon(cfgPath, *verbose, *background)
	case "tui":
		runTUI(cfgPath)
	case "doctor":
		cmdDoctor(cfgPath, *verifyModels)
	case "stop":
		cmdStop()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", sub)
		printHelp()
		os.Exit(1)
	}
}
