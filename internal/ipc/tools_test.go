package ipc

import (
	"testing"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

func shellCall(cmd string) provider.ToolCall {
	return provider.ToolCall{Function: provider.ToolCallFunction{
		Name:      "execute_shell_command",
		Arguments: map[string]any{"command": cmd},
	}}
}

// shellAutoApproved auto-runs only execute_shell_command whose binary is on the
// allowlist; unlisted commands and non-shell tools still require approval.
func TestShellAutoApproved(t *testing.T) {
	if !shellAutoApproved(shellCall("go")) {
		t.Error("allowlisted 'go' should auto-approve")
	}
	if shellAutoApproved(shellCall("curl")) {
		t.Error("unlisted 'curl' must not auto-approve (network egress prompts)")
	}
	if shellAutoApproved(shellCall("")) {
		t.Error("empty command must not auto-approve")
	}
	// a non-shell tool never auto-approves via this path.
	readCall := provider.ToolCall{Function: provider.ToolCallFunction{Name: "read_file"}}
	if shellAutoApproved(readCall) {
		t.Error("non-shell tool must not auto-approve via shellAutoApproved")
	}
}

// SetShellAllowlist replaces (not merges) the auto-approve set; an empty list is
// a no-op so configs predating the shell block keep the built-in default.
func TestSetShellAllowlist(t *testing.T) {
	saved := allowedCommands
	t.Cleanup(func() { allowedCommands = saved })

	SetShellAllowlist([]string{"foo", "bar"})
	if !allowedCommands["foo"] || !allowedCommands["bar"] {
		t.Error("override should install foo and bar")
	}
	if allowedCommands["go"] {
		t.Error("override should replace, not merge, the default set")
	}

	// empty list keeps the current set unchanged.
	SetShellAllowlist(nil)
	if !allowedCommands["foo"] {
		t.Error("empty list should be a no-op")
	}
}
