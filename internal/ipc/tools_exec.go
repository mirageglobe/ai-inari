// tools_exec.go owns builtin tool execution: the execTool dispatcher, the cwd
// sandbox enforcement, and the output/time caps. it does NOT own the tool
// schema definitions or the safe/allowed policy maps (tools.go).

package ipc

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

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
	case "execute_shell_command":
		command, _ := args["command"].(string)
		if command == "" {
			return "", fmt.Errorf("command is required")
		}
		argsStr, _ := args["args"].(string)
		// no allowlist check here: the stream gate already decided auto-run vs.
		// user approval; a command reaching execTool has cleared that gate.
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
	case "find_files":
		safePath, err := sandboxed()
		if err != nil {
			return "", err
		}
		pattern, _ := args["name"].(string)
		if pattern == "" {
			return "", fmt.Errorf("name is required")
		}
		// validate the glob once so a bad pattern is a clean error, not a mid-walk abort.
		if _, err := filepath.Match(pattern, ""); err != nil {
			return "", fmt.Errorf("invalid name pattern: %w", err)
		}
		const maxResults = 200
		var sb strings.Builder
		count := 0
		walkErr := filepath.WalkDir(safePath, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil // skip unreadable entries and directories
			}
			if ok, _ := filepath.Match(pattern, d.Name()); !ok {
				return nil
			}
			rel, _ := filepath.Rel(cwd, p)
			fmt.Fprintf(&sb, "%s\n", rel)
			count++
			if count >= maxResults {
				sb.WriteString("(truncated)\n")
				return filepath.SkipAll
			}
			return nil
		})
		if walkErr != nil {
			return "", walkErr
		}
		return sb.String(), nil
	case "read_lines":
		safePath, err := sandboxed()
		if err != nil {
			return "", err
		}
		start := toInt(args["start"])
		if start < 1 {
			start = 1
		}
		count := toInt(args["count"])
		if count < 1 {
			count = 100 // default window when unset
		}
		const maxCount = 500
		if count > maxCount {
			count = maxCount
		}
		f, err := os.Open(safePath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // tolerate long lines up to 1 MB
		var sb strings.Builder
		line := 0
		for sc.Scan() {
			line++
			if line < start {
				continue
			}
			if line >= start+count {
				break
			}
			fmt.Fprintf(&sb, "%d: %s\n", line, sc.Text())
		}
		if err := sc.Err(); err != nil {
			return "", err
		}
		return sb.String(), nil
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// toInt coerces a tool argument to an int, accepting JSON numbers (float64) and
// numeric strings; anything else yields 0.
func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}

// runUserShell runs a user-authored `!` command line via `sh -c` inside cwd and
// returns combined stdout+stderr. unlike execTool's execute_shell_command (word-split,
// no shell), this is a REAL shell so pipes/globs/redirects work. it is safe because the
// command is typed by the user at their own terminal, not authored by the model, so the
// §8.3 model-injection concern does not apply and no allowlist gate is imposed (the user
// typing the command is the approval). the cwd lock, 30s timeout, and 64KB cap still hold.
func runUserShell(cwd, line string) string {
	ctx, cancel := context.WithTimeout(context.Background(), runCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", line)
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
		return fmt.Sprintf("exit error: %v\n%s", runErr, result)
	}
	return string(result)
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
