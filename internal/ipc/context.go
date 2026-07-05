package ipc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// buildFileTree returns a compact file tree of dir up to maxDepth levels deep.
// directories in skipDirs are pruned. the result is suitable for injection into a system prompt.
func buildFileTree(dir string, maxDepth int) string {
	var sb strings.Builder
	walkTree(&sb, dir, dir, 0, maxDepth)
	return sb.String()
}

func walkTree(sb *strings.Builder, root, current string, depth, maxDepth int) {
	if depth > maxDepth {
		return
	}
	entries, err := os.ReadDir(current)
	if err != nil {
		return
	}
	indent := strings.Repeat("  ", depth)
	for _, e := range entries {
		if skipDirs[e.Name()] {
			continue
		}
		if e.IsDir() {
			fmt.Fprintf(sb, "%s%s/\n", indent, e.Name())
			walkTree(sb, root, filepath.Join(current, e.Name()), depth+1, maxDepth)
		} else {
			fmt.Fprintf(sb, "%s%s\n", indent, e.Name())
		}
	}
}
