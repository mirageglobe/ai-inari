package main

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/ipc"
)

// the suite exists to answer "which builtins go unused", so a builtin with no
// task is a blind spot: every declared tool needs exactly one task aimed at it.
func TestProbeTasksCoverEveryBuiltin(t *testing.T) {
	aimed := make(map[string]int)
	seen := make(map[string]bool)
	for _, task := range probeTasks {
		if task.prompt == "" {
			t.Errorf("task %q has no prompt", task.name)
		}
		if seen[task.name] {
			t.Errorf("duplicate task name %q", task.name)
		}
		seen[task.name] = true
		aimed[task.want]++
	}
	for _, name := range ipc.BuiltinToolNames() {
		if aimed[name] != 1 {
			t.Errorf("builtin %q is the target of %d tasks, want 1", name, aimed[name])
		}
		delete(aimed, name)
	}
	for name := range aimed {
		t.Errorf("task targets unknown tool %q", name)
	}
}

// the fixture must give every task something real to find; a missing file turns
// a tool-selection miss into a fixture artefact.
func TestWriteProbeFixture(t *testing.T) {
	dir := t.TempDir()
	if err := writeProbeFixture(dir); err != nil {
		t.Fatalf("writeProbeFixture: %v", err)
	}
	for _, rel := range []string{"README.md", "Makefile", "go.mod", "main.go", "config.json", filepath.Join("internal", "parse", "parse.go"), filepath.Join("logs", "app.log")} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("missing fixture file %s: %v", rel, err)
		}
	}
	// read_lines is only the right answer if the log is too long to read whole.
	f, err := os.Open(filepath.Join(dir, "logs", "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lines := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines++
	}
	if lines < 200 {
		t.Errorf("app.log has %d lines, want >= 200", lines)
	}
}

func TestProbeResultVerdict(t *testing.T) {
	task := probeTask{name: "t", want: "list_dir"}
	cases := []struct {
		name  string
		calls []probeCall
		want  string
	}{
		{"no calls", nil, verdictNone},
		{"wanted tool", []probeCall{{tool: "list_dir"}}, verdictHit},
		{"wanted after another", []probeCall{{tool: "stat_file"}, {tool: "list_dir"}}, verdictHit},
		{"other tool only", []probeCall{{tool: "execute_shell_command"}}, verdictMiss},
	}
	for _, c := range cases {
		got := probeResult{task: task, calls: c.calls}.verdict()
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestSummariseProbe(t *testing.T) {
	results := []probeResult{
		{task: probeTask{name: "list", want: "list_dir"}, calls: []probeCall{{tool: "list_dir"}}},
		{task: probeTask{name: "find", want: "find_files"}, calls: []probeCall{
			{tool: "execute_shell_command", args: map[string]any{"command": "find"}},
		}},
		{task: probeTask{name: "test", want: "execute_shell_command"}, calls: []probeCall{
			{tool: "execute_shell_command", args: map[string]any{"command": "go"}},
		}},
		{task: probeTask{name: "read", want: "read_file"}, calls: nil},
	}
	s := summariseProbe(results)
	if s.hits != 2 || s.misses != 1 || s.noCall != 1 {
		t.Errorf("hits=%d misses=%d noCall=%d, want 2/1/1", s.hits, s.misses, s.noCall)
	}
	if s.used["execute_shell_command"] != 2 {
		t.Errorf("shell used %d times, want 2", s.used["execute_shell_command"])
	}
	// only the find task fell through: the test task legitimately wants shell.
	if len(s.fellThrough) != 1 || s.fellThrough[0] != "find" {
		t.Errorf("fellThrough=%v, want [find]", s.fellThrough)
	}
	if s.shellCmds["find"] != 1 || s.shellCmds["go"] != 1 {
		t.Errorf("shellCmds=%v", s.shellCmds)
	}
	// unused is derived from the real declaration list, minus what was called.
	unused := make(map[string]bool)
	for _, n := range s.unused {
		unused[n] = true
	}
	if unused["list_dir"] || unused["execute_shell_command"] {
		t.Errorf("called tools reported unused: %v", s.unused)
	}
	if !unused["grep_file"] || !unused["read_lines"] {
		t.Errorf("uncalled tools missing from unused: %v", s.unused)
	}
}
