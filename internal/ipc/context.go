package ipc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mirageglobe/ai-inari/internal/config"
)

// skipDirs are directory names that are always excluded from the file tree.
// these are noise for a model trying to understand a project layout.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	".venv": true, "__pycache__": true, "dist": true, "build": true,
	"bin": true, ".idea": true, ".vscode": true,
}

// agentContextFiles are the project-level context files probed at session creation,
// in priority order. the first one that exists and is non-empty wins.
var agentContextFiles = []string{"AGENTS.md", ".inari/context.md"}

// agentContextCap bounds injected context so a large file cannot blow the prompt budget.
const agentContextCap = 8 * 1024

// readAgentContext returns the contents of the first existing project context file
// under cwd, truncated to agentContextCap. returns "" when none is found or readable,
// so the caller can inject unconditionally.
func readAgentContext(cwd string) string {
	for _, name := range agentContextFiles {
		path, err := sandboxPath(cwd, name)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		if len(content) > agentContextCap {
			content = content[:agentContextCap]
		}
		return content
	}
	return ""
}

// expandUserPath expands a leading ~ (or ~/) to the user's home directory and
// cleans the result, so a session.setcwd path entered by hand behaves like a
// shell path. non-tilde paths are just cleaned; relative paths stay relative
// (resolved against the daemon's own cwd by the subsequent stat).
func expandUserPath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	return filepath.Clean(p)
}

// buildCWDSystemPrompt builds the filesystem-context system prompt for a session
// whose working directory is cwd: a concise-response instruction, the working dir
// and its shallow file tree, the builtin tool descriptions, and (when present) the
// project-level context file. shared by session.create and session.setcwd so both
// entry points inject identical context. cwd must be non-empty.
func buildCWDSystemPrompt(cwd string) string {
	// project overlay (.inari/config.json): its exclude_dirs extend the built-in
	// file-tree skip set for this session only. the overlay's system_prompt is
	// applied by the caller (handleSessionCreate), not here.
	proj := config.LoadProject(cwd)
	tree := buildFileTree(cwd, 3, proj.ExcludeDirs)
	// omit "respond in plain text only" from the default prompt because it conflicts
	// with structured function calling and causes the model to output tool invocations
	// as text rather than structured tool_calls.
	combined := "keep all responses concise and short." +
		"\n\nworking directory: " + cwd + "\n" + tree +
		"\n\nyou have access to the following tools to explore the working directory:\n" +
		"- read_file(path): read the full text of a file\n" +
		"- list_dir(path): list files and directories inside a path\n" +
		"- grep_file(path, pattern): search for a regex pattern across files, returns matching lines\n" +
		"- stat_file(path): return size, modification time, and type for a file or directory\n" +
		"- execute_shell_command(command, args): run a command in the working directory; these run without asking: " + sortedAllowedCommands() + "; any other command asks the user first\n" +
		"use these tools whenever the user asks about files, code, or the project structure."
	// inject a project-level context file (AGENTS.md / .inari/context.md) so the
	// model picks up local conventions without manual copy-paste; absent file is fine.
	if ctx := readAgentContext(cwd); ctx != "" {
		combined += "\n\nproject context:\n" + ctx
	}
	return combined
}

// buildFileTree returns a compact file tree of dir up to maxDepth levels deep.
// directories in skipDirs, plus any names in extraSkip (the project overlay's
// exclude_dirs), are pruned. the result is suitable for injection into a system prompt.
func buildFileTree(dir string, maxDepth int, extraSkip []string) string {
	skip := skipDirs
	if len(extraSkip) > 0 {
		// merge built-in and project skips into a fresh set so the package global
		// is never mutated across sessions.
		skip = make(map[string]bool, len(skipDirs)+len(extraSkip))
		for k := range skipDirs {
			skip[k] = true
		}
		for _, name := range extraSkip {
			skip[name] = true
		}
	}
	var sb strings.Builder
	walkTree(&sb, skip, dir, dir, 0, maxDepth)
	return sb.String()
}

func walkTree(sb *strings.Builder, skip map[string]bool, root, current string, depth, maxDepth int) {
	if depth > maxDepth {
		return
	}
	entries, err := os.ReadDir(current)
	if err != nil {
		return
	}
	indent := strings.Repeat("  ", depth)
	for _, e := range entries {
		if skip[e.Name()] {
			continue
		}
		if e.IsDir() {
			fmt.Fprintf(sb, "%s%s/\n", indent, e.Name())
			walkTree(sb, skip, root, filepath.Join(current, e.Name()), depth+1, maxDepth)
		} else {
			fmt.Fprintf(sb, "%s%s\n", indent, e.Name())
		}
	}
}
