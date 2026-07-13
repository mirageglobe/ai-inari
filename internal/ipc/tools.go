// tools.go owns the builtin tool schema declarations and the execution policy
// maps (safe tools, allowed shell commands). it does NOT own tool execution or
// sandboxing (tools_exec.go).

package ipc

import (
	"sort"
	"strings"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// filesystemTools returns the read-only tool definitions declared to Ollama for
// sessions that have a working directory set. write operations are intentionally absent.
func filesystemTools() []provider.Tool {
	return []provider.Tool{
		{
			Type: "function",
			Function: provider.ToolFunction{
				Name:        "read_file",
				Description: "read the full text content of a file. path must be relative to the session working directory.",
				Parameters: provider.ToolParameters{
					Type: "object",
					Properties: map[string]provider.Property{
						"path": {Type: "string", Description: "relative path to the file"},
					},
					Required: []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: provider.ToolFunction{
				Name:        "list_dir",
				Description: "list the names of files and directories inside a directory. path must be relative to the session working directory.",
				Parameters: provider.ToolParameters{
					Type: "object",
					Properties: map[string]provider.Property{
						"path": {Type: "string", Description: "relative path to the directory; use \".\" for the root"},
					},
					Required: []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: provider.ToolFunction{
				Name:        "grep_file",
				Description: "search for a regex pattern in files under a directory. returns matching lines with file path and line number. path must be relative to the session working directory.",
				Parameters: provider.ToolParameters{
					Type: "object",
					Properties: map[string]provider.Property{
						"path":    {Type: "string", Description: "relative path to the directory to search; use \".\" for the root"},
						"pattern": {Type: "string", Description: "regular expression to search for"},
					},
					Required: []string{"path", "pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: provider.ToolFunction{
				Name:        "stat_file",
				Description: "return metadata for a file or directory: size in bytes, modification time, and whether it is a directory. path must be relative to the session working directory.",
				Parameters: provider.ToolParameters{
					Type: "object",
					Properties: map[string]provider.Property{
						"path": {Type: "string", Description: "relative path to the file or directory"},
					},
					Required: []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: provider.ToolFunction{
				Name:        "execute_shell_command",
				Description: "run a shell command inside the session working directory and return its stdout and stderr. commands on the auto-approve allowlist run immediately; any other command runs only after the user approves it, so prefer allowlisted commands.",
				Parameters: provider.ToolParameters{
					Type: "object",
					Properties: map[string]provider.Property{
						"command": {Type: "string", Description: "the binary to run (e.g. \"go\", \"make\", \"git\")"},
						"args":    {Type: "string", Description: "space-separated arguments (e.g. \"test ./...\")"},
					},
					Required: []string{"command"},
				},
			},
		},
	}
}

// safeTools are executed immediately without an approval round-trip.
// read-only filesystem tools pose no write/exec risk, so interrupting the user for every call would be noise.
// any tool not in this set, currently only "execute_shell_command", always requires inari approval.
var safeTools = map[string]bool{
	"read_file": true,
	"list_dir":  true,
	"grep_file": true,
	"stat_file": true,
}

// defaultShellAllowlist is the built-in set of command binaries that
// execute_shell_command runs without a per-call approval prompt. read/build/
// inspect commands only; network commands (curl, wget) are intentionally absent
// so they still prompt. config.json "shell.allowlist" overrides this set.
var defaultShellAllowlist = []string{
	"go", "make", "git", "ls", "cat", "find", "pwd", "whoami",
	"uname", "wc", "date", "echo", "which", "df", "du", "uptime", "ps",
}

// allowedCommands is the active auto-approve set: commands here run without a
// prompt; anything else still requires user approval. seeded from the default
// and replaceable via SetShellAllowlist at startup. base names only; args are
// passed separately and never shell-expanded.
var allowedCommands = commandSet(defaultShellAllowlist)

func commandSet(cmds []string) map[string]bool {
	m := make(map[string]bool, len(cmds))
	for _, c := range cmds {
		m[c] = true
	}
	return m
}

// SetShellAllowlist replaces the auto-approve command set from config. it must
// be called before the server starts accepting connections. an empty list keeps
// the built-in default (covers configs predating the shell.allowlist field).
func SetShellAllowlist(cmds []string) {
	if len(cmds) == 0 {
		return
	}
	allowedCommands = commandSet(cmds)
}

// shellAutoApproved reports whether a tool call may run without a user prompt:
// only execute_shell_command whose binary is on the allowlist. every other
// non-safe tool returns false and still requires explicit approval.
func shellAutoApproved(tc provider.ToolCall) bool {
	if tc.Function.Name != "execute_shell_command" {
		return false
	}
	command, _ := tc.Function.Arguments["command"].(string)
	return allowedCommands[command]
}

// sortedAllowedCommands returns the allowed command names as a sorted, comma-separated string.
func sortedAllowedCommands() string {
	cmds := make([]string, 0, len(allowedCommands))
	for k := range allowedCommands {
		cmds = append(cmds, k)
	}
	sort.Strings(cmds)
	return strings.Join(cmds, ", ")
}
