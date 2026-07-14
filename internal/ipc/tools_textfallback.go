// tools_textfallback.go owns the prompt-based tool-call fallback: parsing a tool
// invocation a model wrote as TEXT (instead of emitting a native tool_call) so the
// stream loop can dispatch it through the normal execTool gate. it does NOT own the
// tool schema (tools.go) or execution (tools_exec.go).

package ipc

import (
	"regexp"
	"strings"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// textToolCallRe finds a `name(...)` or `name{...}` invocation anywhere in text.
// args never nest braces in our schema, so a non-greedy flat capture is enough.
var textToolCallRe = regexp.MustCompile(`([a-z_]+)\s*[\({]([^)}]*)[\)}]`)

// textToolArgRe extracts `key: "value"`, `key='value'`, or `key=bareword` pairs.
var textToolArgRe = regexp.MustCompile(`([a-zA-Z_]+)\s*[:=]\s*(?:"([^"]*)"|'([^']*)'|([^,}\)]+))`)

// parseTextToolCall scans assistant text for a tool invocation the model wrote as
// text rather than emitting a native tool_call (small models at high temperature do
// this, e.g. `list_dir{path: "."}` or `read_file(path='README.md')`). it matches
// only names in known and only when at least one arg pair is present, so prose that
// merely mentions a tool ("use list_dir to browse") is not dispatched. the leftmost
// qualifying match wins. returns ok=false when no tool call is found.
//
// fenced code blocks (```bash ls```) are intentionally NOT parsed: a model showing
// an example command is ambiguous versus intent, and mis-dispatching it would run
// something the user only wanted explained.
func parseTextToolCall(content string, known map[string]bool) (provider.ToolCall, bool) {
	for _, m := range textToolCallRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		if !known[name] {
			continue
		}
		args := parseTextToolArgs(m[2])
		if len(args) == 0 {
			continue // require an arg pair; filters bare prose mentions
		}
		return provider.ToolCall{Function: provider.ToolCallFunction{Name: name, Arguments: args}}, true
	}
	return provider.ToolCall{}, false
}

// parseTextToolArgs pulls string key/value pairs out of an invocation's arg body.
// every builtin tool parameter is a string, so string values are sufficient.
func parseTextToolArgs(inner string) map[string]any {
	args := map[string]any{}
	for _, m := range textToolArgRe.FindAllStringSubmatch(inner, -1) {
		val := m[2]
		if val == "" {
			val = m[3]
		}
		if val == "" {
			val = strings.TrimSpace(m[4])
		}
		args[m[1]] = val
	}
	return args
}
