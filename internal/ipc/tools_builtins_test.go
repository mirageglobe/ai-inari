package ipc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// find_files matches names by glob recursively and stays inside the cwd sandbox.
func TestExecFindFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.go", "b.txt", filepath.Join("sub", "c.go")} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out, err := execTool("find_files", map[string]any{"path": ".", "name": "*.go"}, dir)
	if err != nil {
		t.Fatalf("find_files: %v", err)
	}
	if !strings.Contains(out, "a.go") || !strings.Contains(out, filepath.Join("sub", "c.go")) {
		t.Errorf("expected a.go and sub/c.go, got %q", out)
	}
	if strings.Contains(out, "b.txt") {
		t.Errorf("glob *.go must not match b.txt, got %q", out)
	}
	if _, err := execTool("find_files", map[string]any{"path": "."}, dir); err == nil {
		t.Error("missing name should error")
	}
	if _, err := execTool("find_files", map[string]any{"path": "../..", "name": "*"}, dir); err == nil {
		t.Error("path escaping the sandbox should error")
	}
}

// read_lines returns a numbered line range and accepts numeric-string args.
func TestExecReadLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "log.txt"), []byte("one\ntwo\nthree\nfour\nfive\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := execTool("read_lines", map[string]any{"path": "log.txt", "start": float64(2), "count": float64(2)}, dir)
	if err != nil {
		t.Fatalf("read_lines: %v", err)
	}
	if out != "2: two\n3: three\n" {
		t.Errorf("start=2 count=2 got %q", out)
	}
	// numeric strings are accepted (models often send them as strings).
	if out, _ = execTool("read_lines", map[string]any{"path": "log.txt", "start": "1", "count": "1"}, dir); out != "1: one\n" {
		t.Errorf("string args got %q", out)
	}
	// start past EOF is empty, not an error.
	if out, err = execTool("read_lines", map[string]any{"path": "log.txt", "start": float64(99)}, dir); err != nil || out != "" {
		t.Errorf("start past EOF: out=%q err=%v", out, err)
	}
}

// awk/sed/jq are on the default auto-approve allowlist for stream filtering.
func TestAwkSedJqAllowlisted(t *testing.T) {
	for _, cmd := range []string{"awk", "sed", "jq"} {
		if !shellAutoApproved(shellCall(cmd)) {
			t.Errorf("%q should auto-approve from the default allowlist", cmd)
		}
	}
}
