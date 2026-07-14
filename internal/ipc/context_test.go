package ipc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildCWDSystemPromptGuard asserts the prompt keeps the file tree for
// orientation but frames it as stale and directs the model to call tools rather
// than answer from it, and that the old call-shaped `name(args)` tool list (which
// nudged the model to reproduce tool calls as text) is gone.
func TestBuildCWDSystemPromptGuard(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}

	p := buildCWDSystemPrompt(dir)

	// tree retained for orientation.
	if !strings.Contains(p, "working directory: "+dir) {
		t.Fatalf("working directory line missing, got %q", p)
	}
	if !strings.Contains(p, "src/") {
		t.Fatalf("file tree should still be present, got %q", p)
	}
	// guard directive present.
	for _, want := range []string{"snapshot", "never answer", "always call a tool"} {
		if !strings.Contains(p, want) {
			t.Fatalf("guard directive missing %q, got %q", want, p)
		}
	}
	// tool names named plainly, but not in the old call-shaped prose form.
	if !strings.Contains(p, "list_dir") || !strings.Contains(p, "execute_shell_command") {
		t.Fatalf("tool names should be mentioned, got %q", p)
	}
	for _, banned := range []string{"read_file(path)", "list_dir(path)", "execute_shell_command(command, args)"} {
		if strings.Contains(p, banned) {
			t.Fatalf("call-shaped tool prose %q should be removed, got %q", banned, p)
		}
	}
}
