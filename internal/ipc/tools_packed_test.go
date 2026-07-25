package ipc

import (
	"strings"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// measured against gemma4:e2b via `inari probe`: the model sometimes packs the
// whole command line into "command" ({"command":"make test"}) instead of splitting
// it across command/args. that form used to miss the allowlist and then fail at
// exec, so an allowlisted command prompted the user and still could not run.
func TestSplitShellCommand(t *testing.T) {
	cases := []struct {
		command, argsStr string
		wantBin          string
		wantArgs         []string
	}{
		{"make", "", "make", nil},
		{"make", "test", "make", []string{"test"}},
		{"make test", "", "make", []string{"test"}},
		{"go test ./...", "", "go", []string{"test", "./..."}},
		{"  go   test   ./...  ", "", "go", []string{"test", "./..."}},
		// a packed command plus a separate args field: packed args come first so
		// the command line reads in the order the model wrote it.
		{"go test", "./...", "go", []string{"test", "./..."}},
		{"", "test", "", nil},
	}
	for _, c := range cases {
		bin, args := splitShellCommand(c.command, c.argsStr)
		if bin != c.wantBin {
			t.Errorf("%q/%q: binary = %q want %q", c.command, c.argsStr, bin, c.wantBin)
		}
		if strings.Join(args, " ") != strings.Join(c.wantArgs, " ") {
			t.Errorf("%q/%q: args = %v want %v", c.command, c.argsStr, args, c.wantArgs)
		}
	}
}

// the allowlist gate must see the real binary, not the packed string, otherwise an
// allowlisted command is treated as unlisted.
func TestShellAutoApprovedPackedCommand(t *testing.T) {
	packed := func(cmd string) provider.ToolCall {
		return provider.ToolCall{Function: provider.ToolCallFunction{
			Name:      "execute_shell_command",
			Arguments: map[string]any{"command": cmd},
		}}
	}
	if !shellAutoApproved(packed("make test")) {
		t.Error("packed allowlisted command should auto-approve")
	}
	if !shellAutoApproved(packed("go test ./...")) {
		t.Error("packed allowlisted command with args should auto-approve")
	}
	// normalising must not widen the gate: an unlisted binary still prompts,
	// and a shell metacharacter never becomes an allowlisted binary.
	if shellAutoApproved(packed("curl http://example.com")) {
		t.Error("packed unlisted command must not auto-approve")
	}
	if shellAutoApproved(packed("go; curl http://example.com")) {
		t.Error("metacharacter-joined command must not auto-approve")
	}
}

// end to end: a packed command actually runs instead of failing with
// "executable file not found".
func TestExecToolPackedCommandRuns(t *testing.T) {
	out, err := execTool("execute_shell_command", map[string]any{"command": "echo hi there"}, t.TempDir())
	if err != nil {
		t.Fatalf("execTool: %v", err)
	}
	if strings.TrimSpace(out) != "hi there" {
		t.Errorf("output = %q want %q", strings.TrimSpace(out), "hi there")
	}
}
