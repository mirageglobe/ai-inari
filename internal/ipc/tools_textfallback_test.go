package ipc

import "testing"

// known mirrors the builtin tool names offered when a session has a cwd.
var known = map[string]bool{
	"read_file": true, "list_dir": true, "grep_file": true,
	"stat_file": true, "execute_shell_command": true,
}

func TestParseTextToolCall(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		wantOK   bool
		wantName string
		wantArgs map[string]string
	}{
		// forms observed from gemma4:e2b when it narrates instead of calling
		{"brace json-ish", `list_dir{path: "."}`, true, "list_dir", map[string]string{"path": "."}},
		{"paren single-quote", `read_file(path='README.md')`, true, "read_file", map[string]string{"path": "README.md"}},
		{"shell brace", `execute_shell_command{command: "ls"}`, true, "execute_shell_command", map[string]string{"command": "ls"}},
		{"embedded in sentence", `sure, I'll run list_dir{path: "src"} now`, true, "list_dir", map[string]string{"path": "src"}},
		{"two args", `grep_file(path=".", pattern="TODO")`, true, "grep_file", map[string]string{"path": ".", "pattern": "TODO"}},
		// must NOT dispatch
		{"prose mention only", `use list_dir to browse the directory`, false, "", nil},
		{"unknown tool name", `frobnicate(x="1")`, false, "", nil},
		{"no args", `list_dir()`, false, "", nil},
		{"plain reply", `the project is a Go TUI for Ollama`, false, "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc, ok := parseTextToolCall(c.content, known)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (content %q)", ok, c.wantOK, c.content)
			}
			if !ok {
				return
			}
			if tc.Function.Name != c.wantName {
				t.Fatalf("name = %q, want %q", tc.Function.Name, c.wantName)
			}
			for k, want := range c.wantArgs {
				if got, _ := tc.Function.Arguments[k].(string); got != want {
					t.Fatalf("arg %q = %q, want %q", k, got, want)
				}
			}
			if len(tc.Function.Arguments) != len(c.wantArgs) {
				t.Fatalf("arg count = %d, want %d (%v)", len(tc.Function.Arguments), len(c.wantArgs), tc.Function.Arguments)
			}
		})
	}
}

// TestParseTextToolCallEmptyKnown asserts nothing dispatches when no tools are
// offered (session without a cwd).
func TestParseTextToolCallEmptyKnown(t *testing.T) {
	if _, ok := parseTextToolCall(`list_dir{path: "."}`, map[string]bool{}); ok {
		t.Fatal("should not match when no tools are known")
	}
}
