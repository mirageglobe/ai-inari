package views

import (
	"strings"
	"testing"

	"github.com/mirageglobe/ai-inari/internal/provider"
)

// a zero-value chat has no model, no cwd, and no assistant reply, so the three
// context-gated commands must report disabled.
func TestMatchingCommandsDisabledWhenEmpty(t *testing.T) {
	c := Chat{}
	got := map[string]bool{}
	for _, s := range c.matchingCommands("/") {
		got[s.Name] = s.Enabled
	}
	for _, name := range []string{"/model unload", "/copy", "/tools"} {
		if got[name] {
			t.Errorf("%q should be disabled on an empty chat", name)
		}
	}
	if !got["/clear"] {
		t.Error("/clear has no predicate and should always be enabled")
	}
}

// once a model, cwd, and assistant reply exist, the gated commands enable.
func TestMatchingCommandsEnabledWhenReady(t *testing.T) {
	c := Chat{
		model:    "gemma4:e2b",
		cwd:      "/tmp/proj",
		messages: []provider.Message{{Role: "assistant", Content: "hi"}},
	}
	got := map[string]bool{}
	for _, s := range c.matchingCommands("/") {
		got[s.Name] = s.Enabled
	}
	for _, name := range []string{"/model unload", "/copy", "/tools"} {
		if !got[name] {
			t.Errorf("%q should be enabled when its context is present", name)
		}
	}
}

// the "/model" prefix must match both /model and /model unload, and nothing else.
func TestMatchingCommandsPrefixFilter(t *testing.T) {
	c := Chat{}
	var names []string
	for _, s := range c.matchingCommands("/model") {
		names = append(names, s.Name)
	}
	want := []string{"/model", "/model unload"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("prefix /model matched %v, want %v", names, want)
	}
}

// the help overlay's chat rows are derived from the table, so every command
// name must appear as a help key. guards against the two lists drifting apart.
func TestChatHelpCoversTable(t *testing.T) {
	keys := map[string]bool{}
	for _, e := range helpByView["chat"] {
		keys[e.key] = true
	}
	for _, cmd := range chatCommandTable {
		if !keys["["+cmd.Name+"]"] {
			t.Errorf("command %q missing from chat help overlay", cmd.Name)
		}
	}
}

// every command in the table must be handled by the dispatch switch; an
// unhandled command falls through to the "unknown command" warning. guards
// against a command being added to the table but not to handleSlashCommand.
func TestEveryCommandDispatches(t *testing.T) {
	c := Chat{}
	for _, cmd := range chatCommandTable {
		got, _ := c.handleSlashCommand(cmd.Name)
		ch, ok := got.(Chat)
		if ok && strings.HasPrefix(ch.status, "[warn] unknown command") {
			t.Errorf("command %q is in the table but not handled by dispatch", cmd.Name)
		}
	}
}
