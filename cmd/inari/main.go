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
