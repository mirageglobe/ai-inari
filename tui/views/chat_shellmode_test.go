package views

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

var (
	keyBang      = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")}
	keyBackspace = tea.KeyMsg{Type: tea.KeyBackspace}
	keyEnter     = tea.KeyMsg{Type: tea.KeyEnter}
)

// newShellModeChat builds the minimum Chat needed to drive handleKey: a real
// textarea, since the mode transitions all key off whether the input is empty.
func newShellModeChat(t *testing.T, value string, shellMode bool) Chat {
	t.Helper()
	ta := textarea.New()
	ta.SetValue(value)
	return Chat{input: ta, shellMode: shellMode}
}

// TestShellModeEntersOnBangAtEmptyInput asserts `!` at an empty input switches
// mode and is consumed rather than typed, so it never lands in the buffer.
func TestShellModeEntersOnBangAtEmptyInput(t *testing.T) {
	c := newShellModeChat(t, "", false)
	nc, _, handled := c.handleKey(keyBang)
	if !handled {
		t.Fatal("bang at an empty input should be handled, not passed to the textarea")
	}
	if !nc.shellMode {
		t.Error("shellMode: want true after bang at an empty input")
	}
	if nc.input.Value() != "" {
		t.Errorf("input = %q, want empty: the bang must be consumed, not inserted", nc.input.Value())
	}
}

// a bang mid-line is an ordinary character (echo hi!), so it must fall through to
// the textarea and leave the mode alone.
func TestBangMidLineIsJustACharacter(t *testing.T) {
	c := newShellModeChat(t, "echo hi", false)
	nc, _, handled := c.handleKey(keyBang)
	if handled {
		t.Error("bang with text present should fall through so the textarea types it")
	}
	if nc.shellMode {
		t.Error("shellMode: want false, a mid-line bang must not switch mode")
	}
}

// TestShellModeExitsOnBackspaceAtEmptyInput is the documented way out.
func TestShellModeExitsOnBackspaceAtEmptyInput(t *testing.T) {
	c := newShellModeChat(t, "", true)
	nc, _, handled := c.handleKey(keyBackspace)
	if !handled {
		t.Fatal("backspace at an empty input in shell mode should be handled")
	}
	if nc.shellMode {
		t.Error("shellMode: want false after backspace at an empty input")
	}
}

// the sharp edge: backspace while editing must delete a character, never drop the
// mode, or a typo mid-command would silently send the next line to the model.
func TestBackspaceWithTextKeepsShellMode(t *testing.T) {
	c := newShellModeChat(t, "ls -la", true)
	nc, _, handled := c.handleKey(keyBackspace)
	if handled {
		t.Error("backspace with text present should fall through to the textarea")
	}
	if !nc.shellMode {
		t.Error("shellMode: want true, editing mid-command must not exit the mode")
	}
}

// backspace outside shell mode is an ordinary edit key even at an empty input.
func TestBackspaceOutsideShellModeFallsThrough(t *testing.T) {
	c := newShellModeChat(t, "", false)
	_, _, handled := c.handleKey(keyBackspace)
	if handled {
		t.Error("backspace at an empty input outside shell mode should fall through")
	}
}

// TestShellModePersistsAcrossSubmit is the whole point of the change: before it,
// input.Reset() cleared the `!` along with the line, so the mode died every turn.
func TestShellModePersistsAcrossSubmit(t *testing.T) {
	c := newShellModeChat(t, "ls", true)
	nc, cmd, handled := c.handleKey(keyEnter)
	if !handled {
		t.Fatal("enter in shell mode should be handled")
	}
	if cmd == nil {
		t.Error("enter in shell mode should return a command that runs the line")
	}
	if !nc.shellMode {
		t.Error("shellMode: want true after submitting; a second command needs no prefix")
	}
	if nc.input.Value() != "" {
		t.Errorf("input = %q, want empty after submit", nc.input.Value())
	}
}

// a pasted line keeps working through the old prefix route, which never enters the
// mode because the paste is not a single bang keypress at an empty input.
func TestBangPrefixStillRunsWithoutTheMode(t *testing.T) {
	c := newShellModeChat(t, "!echo hi", false)
	nc, cmd, handled := c.handleKey(keyEnter)
	if !handled || cmd == nil {
		t.Fatal("a bang-prefixed line should still run as a shell command")
	}
	if nc.shellMode {
		t.Error("shellMode: want false, the prefix route does not switch mode")
	}
}

// the prompt is the only signal the user has for which mode they are in, so it must
// follow the mode and not merely the buffer contents.
func TestInputPromptFollowsShellMode(t *testing.T) {
	cases := []struct {
		name string
		chat Chat
		want string
	}{
		{"shell mode, empty input", newShellModeChat(t, "", true), "[sh] ❯ "},
		{"shell mode wins over a slash", newShellModeChat(t, "/help", true), "[sh] ❯ "},
		{"bang prefix without the mode", newShellModeChat(t, "!ls", false), "[sh] ❯ "},
		{"slash command", newShellModeChat(t, "/help", false), "[cmd] ❯ "},
		{"plain chat", newShellModeChat(t, "hello", false), "[chat] ❯ "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.chat.inputPrompt(); got != tc.want {
				t.Errorf("inputPrompt() = %q, want %q", got, tc.want)
			}
		})
	}
}
