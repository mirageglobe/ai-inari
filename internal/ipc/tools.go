package ipc

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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
				Name:        "run",
				Description: "run an allowlisted shell command inside the session working directory and return its stdout and stderr. only specific commands are permitted; others are rejected.",
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
// any tool not in this set, currently only "run", always requires kitsune approval.
var safeTools = map[string]bool{
	"read_file": true,
	"list_dir":  true,
	"grep_file": true,
	"stat_file": true,
}

// allowedCommands is the set of binaries that run may invoke.
// each entry is the base command name only; arguments are passed separately and never shell-expanded.
var allowedCommands = map[string]bool{
	"go":     true,
	"make":   true,
	"git":    true,
	"date":   true,
	"echo":   true,
	"pwd":    true,
	"whoami": true,
	"uname":  true,
	"wc":     true,
	"curl":   true,
	"wget":   true,
	"find":   true,
	"ps":     true,
	"ls":     true,
	"cat":    true,
	"df":     true,
	"du":     true,
	"uptime": true,
	"which":  true,
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

const runCommandTimeout = 30 * time.Second
const runCommandMaxBytes = 64 * 1024 // 64 KB output cap

// execTool dispatches a tool call by name, enforces the cwd sandbox, and returns the result.
func execTool(name string, args map[string]any, cwd string) (string, error) {
	sandboxed := func() (string, error) {
		rawPath, _ := args["path"].(string)
		return sandboxPath(cwd, rawPath)
	}
	switch name {
	case "read_file":
		safePath, err := sandboxed()
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(safePath)
		if err != nil {
			return "", err
		}
		const maxBytes = 1024 * 1024 // 1 MB blast-radius cap
		if len(data) > maxBytes {
			data = data[:maxBytes]
		}
		return string(data), nil
	case "list_dir":
		safePath, err := sandboxed()
		if err != nil {
			return "", err
		}
		entries, err := os.ReadDir(safePath)
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		for _, e := range entries {
			if e.IsDir() {
				fmt.Fprintf(&sb, "%s/\n", e.Name())
			} else {
				fmt.Fprintf(&sb, "%s\n", e.Name())
			}
		}
		return sb.String(), nil
	case "grep_file":
		safePath, err := sandboxed()
		if err != nil {
			return "", err
		}
		rawPattern, _ := args["pattern"].(string)
		re, err := regexp.Compile(rawPattern)
		if err != nil {
			return "", fmt.Errorf("invalid pattern: %w", err)
		}
		const maxMatches = 200
		var sb strings.Builder
		count := 0
		walkErr := filepath.WalkDir(safePath, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil // skip unreadable files
			}
			rel, _ := filepath.Rel(cwd, p)
			for i, line := range strings.Split(string(data), "\n") {
				if re.MatchString(line) {
					fmt.Fprintf(&sb, "%s:%d: %s\n", rel, i+1, line)
					count++
					if count >= maxMatches {
						sb.WriteString("(truncated)\n")
						return filepath.SkipAll
					}
				}
			}
			return nil
		})
		if walkErr != nil {
			return "", walkErr
		}
		return sb.String(), nil
	case "stat_file":
		rawPath, _ := args["path"].(string)
		safePath, err := sandboxed()
		if err != nil {
			return "", err
		}
		info, err := os.Stat(safePath)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("path: %s\nsize: %d bytes\nmodified: %s\nis_dir: %v\n",
			rawPath, info.Size(), info.ModTime().Format(time.RFC3339), info.IsDir()), nil
	case "run":
		command, _ := args["command"].(string)
		argsStr, _ := args["args"].(string)
		if !allowedCommands[command] {
			return "", fmt.Errorf("command not allowed: %q; permitted: go, make, git", command)
		}
		var cmdArgs []string
		if argsStr != "" {
			cmdArgs = strings.Fields(argsStr)
		}
		ctx, cancel := context.WithTimeout(context.Background(), runCommandTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, command, cmdArgs...)
		cmd.Dir = cwd
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		runErr := cmd.Run()
		result := out.Bytes()
		if len(result) > runCommandMaxBytes {
			result = append(result[:runCommandMaxBytes], []byte("\n(truncated)")...)
		}
		if runErr != nil {
			return fmt.Sprintf("exit error: %v\n%s", runErr, result), nil
		}
		return string(result), nil
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// sandboxPath resolves p relative to cwd and rejects any path that escapes the root.
func sandboxPath(cwd, p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path is required")
	}
	abs := filepath.Join(cwd, filepath.FromSlash(p))
	rel, err := filepath.Rel(cwd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes working directory")
	}
	return abs, nil
}
