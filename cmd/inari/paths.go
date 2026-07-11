package main

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
