package ipc

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// the fixture tree under testdata/fixture is a contract: these literals must match
// the files byte for byte, so editing one without the other fails loudly rather
// than quietly weakening the assertions.
const (
	fixtureReadme   = "# fixture\n\nthis tree is a test contract; see the direct tool tests.\n"
	fixtureSettings = "{\n  \"name\": \"fixture\"\n}\n"
	fixtureSample   = "alpha\nbeta widget\ngamma\n"
)

// fixtureDir returns the fixture tree as an absolute path, which is what execTool
// expects as its cwd. tests run in the package directory, so testdata is relative.
func fixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatalf("resolve fixture dir: %v", err)
	}
	return dir
}

func TestExecToolReadFile(t *testing.T) {
	dir := fixtureDir(t)

	out, err := execTool("read_file", map[string]any{"path": "README.md"}, dir)
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if out != fixtureReadme {
		t.Errorf("read_file README.md = %q, want %q", out, fixtureReadme)
	}

	// a nested path must resolve through the sandbox join, not just a flat name.
	if out, err = execTool("read_file", map[string]any{"path": "notes/sample.txt"}, dir); err != nil {
		t.Fatalf("read_file nested: %v", err)
	} else if out != fixtureSample {
		t.Errorf("read_file notes/sample.txt = %q, want %q", out, fixtureSample)
	}

	if _, err = execTool("read_file", map[string]any{"path": "nope.txt"}, dir); err == nil {
		t.Error("read_file on a missing path: want an error")
	}
	if _, err = execTool("read_file", map[string]any{"path": "../../go.mod"}, dir); err == nil {
		t.Error("read_file escaping the cwd: want an error")
	}
	if _, err = execTool("read_file", map[string]any{}, dir); err == nil {
		t.Error("read_file with no path: want an error")
	}
}

func TestExecToolListDir(t *testing.T) {
	dir := fixtureDir(t)

	// os.ReadDir sorts by name, and directories are marked with a trailing slash,
	// so the whole listing is deterministic and asserted exactly.
	want := "README.md\ninternal/\nnotes/\nsettings.json\n"
	out, err := execTool("list_dir", map[string]any{"path": "."}, dir)
	if err != nil {
		t.Fatalf("list_dir: %v", err)
	}
	if out != want {
		t.Errorf("list_dir . = %q, want %q", out, want)
	}

	if out, err = execTool("list_dir", map[string]any{"path": "internal"}, dir); err != nil {
		t.Fatalf("list_dir nested: %v", err)
	} else if out != "parse/\n" {
		t.Errorf("list_dir internal = %q, want %q", out, "parse/\n")
	}

	if _, err = execTool("list_dir", map[string]any{"path": "README.md"}, dir); err == nil {
		t.Error("list_dir on a file: want an error")
	}
	if _, err = execTool("list_dir", map[string]any{"path": "../.."}, dir); err == nil {
		t.Error("list_dir escaping the cwd: want an error")
	}
}

func TestExecToolGrepFile(t *testing.T) {
	dir := fixtureDir(t)

	// grep_file walks recursively and reports paths relative to cwd. "fixture"
	// appears once in README.md and once in settings.json, and WalkDir is lexical,
	// so both the matches and their order are fixed.
	want := "README.md:1: # fixture\nsettings.json:2:   \"name\": \"fixture\"\n"
	out, err := execTool("grep_file", map[string]any{"path": ".", "pattern": "fixture"}, dir)
	if err != nil {
		t.Fatalf("grep_file: %v", err)
	}
	if out != want {
		t.Errorf("grep_file = %q, want %q", out, want)
	}

	// a pattern matching nothing is not an error, it is an empty result.
	if out, err = execTool("grep_file", map[string]any{"path": ".", "pattern": "zzz"}, dir); err != nil {
		t.Fatalf("grep_file no match: %v", err)
	} else if out != "" {
		t.Errorf("grep_file with no match = %q, want empty", out)
	}

	if _, err = execTool("grep_file", map[string]any{"path": ".", "pattern": "("}, dir); err == nil {
		t.Error("grep_file with an invalid regex: want an error")
	}
	if _, err = execTool("grep_file", map[string]any{"path": "../..", "pattern": "x"}, dir); err == nil {
		t.Error("grep_file escaping the cwd: want an error")
	}
}

func TestExecToolStatFile(t *testing.T) {
	dir := fixtureDir(t)

	out, err := execTool("stat_file", map[string]any{"path": "settings.json"}, dir)
	if err != nil {
		t.Fatalf("stat_file: %v", err)
	}
	// stat_file echoes the path as the model supplied it, not the resolved one.
	for _, want := range []string{
		"path: settings.json\n",
		fmt.Sprintf("size: %d bytes\n", len(fixtureSettings)),
		"is_dir: false\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stat_file output missing %q, got %q", want, out)
		}
	}

	// the modified line must be machine-readable; a bad layout would still "look"
	// fine in a chat reply, so it is parsed rather than eyeballed.
	var modified string
	for _, line := range strings.Split(out, "\n") {
		if after, ok := strings.CutPrefix(line, "modified: "); ok {
			modified = after
		}
	}
	if modified == "" {
		t.Fatalf("stat_file output has no modified line: %q", out)
	}
	if _, err = time.Parse(time.RFC3339, modified); err != nil {
		t.Errorf("modified %q is not RFC3339: %v", modified, err)
	}

	if out, err = execTool("stat_file", map[string]any{"path": "notes"}, dir); err != nil {
		t.Fatalf("stat_file on a dir: %v", err)
	} else if !strings.Contains(out, "is_dir: true\n") {
		t.Errorf("stat_file notes = %q, want is_dir: true", out)
	}

	if _, err = execTool("stat_file", map[string]any{"path": "nope.txt"}, dir); err == nil {
		t.Error("stat_file on a missing path: want an error")
	}
	if _, err = execTool("stat_file", map[string]any{"path": "../../go.mod"}, dir); err == nil {
		t.Error("stat_file escaping the cwd: want an error")
	}
}
