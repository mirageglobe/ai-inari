package ipc

import "testing"

// the probe audits tool selection against BuiltinToolNames, so it must stay the
// authoritative list: every declared tool, in declaration order, nothing invented.
func TestBuiltinToolNamesMatchDeclarations(t *testing.T) {
	names := BuiltinToolNames()
	tools := filesystemTools()
	if len(names) != len(tools) {
		t.Fatalf("names=%d tools=%d", len(names), len(tools))
	}
	for i, tool := range tools {
		if names[i] != tool.Function.Name {
			t.Errorf("index %d: got %q want %q", i, names[i], tool.Function.Name)
		}
	}
	if len(names) == 0 {
		t.Fatal("no builtin tools declared")
	}
}

// every safe tool must be a declared tool; a stale entry would grant auto-execute
// to a name the model can never call, hiding a rename.
func TestSafeToolsAreDeclared(t *testing.T) {
	declared := make(map[string]bool)
	for _, n := range BuiltinToolNames() {
		declared[n] = true
	}
	for name := range safeTools {
		if !declared[name] {
			t.Errorf("safeTools has undeclared tool %q", name)
		}
	}
}
