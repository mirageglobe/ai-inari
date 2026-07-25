package main

import (
	"os"
	"path/filepath"
	"testing"
)

// the probe needs the whole call chain for a turn, not just the first tool, so
// the scan must return every tool.call for the session in log order and ignore
// other sessions, other methods, and anything written before the offset.
func TestSessionToolCallsReturnsChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	before := `{"ts":"t0","method":"tool.call","params":{"session":"aaa","tool":"read_file"}}` + "\n"
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	from := fileSize(path)

	rest := `{"ts":"t1","method":"session.stream","params":{"id":"aaa"}}
{"ts":"t2","method":"tool.call","params":{"session":"aaa","tool":"list_dir","args":{"path":"."}}}
{"ts":"t3","method":"tool.call","params":{"session":"bbb","tool":"stat_file"}}
not json at all
{"ts":"t4","method":"tool.call","params":{"session":"aaa","tool":"execute_shell_command","args":{"command":"go","args":"test ./..."},"failed":true}}
{"ts":"t5","method":"tool.denied","params":{"session":"aaa","tool":"execute_shell_command","args":{"command":"curl"}}}
`
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(rest); err != nil {
		t.Fatal(err)
	}
	f.Close()

	calls := sessionToolCalls(path, from, "aaa")
	if len(calls) != 3 {
		t.Fatalf("got %d calls, want 3: %+v", len(calls), calls)
	}
	if calls[0].tool != "list_dir" || calls[0].args["path"] != "." {
		t.Errorf("call 0 = %+v", calls[0])
	}
	if calls[1].tool != "execute_shell_command" || !calls[1].failed {
		t.Errorf("call 1 = %+v", calls[1])
	}
	if calls[1].args["command"] != "go" {
		t.Errorf("call 1 args = %v", calls[1].args)
	}
	// a refused call is part of the chain the probe reports, flagged as denied.
	if calls[2].tool != "execute_shell_command" || !calls[2].denied {
		t.Errorf("call 2 = %+v, want a denied shell call", calls[2])
	}
	if calls[0].denied || calls[1].denied {
		t.Error("executed calls must not be flagged denied")
	}

	// doctor's first-tool check keeps working off the same scan.
	if got := sessionToolCalled(path, from, "aaa"); got != "list_dir" {
		t.Errorf("sessionToolCalled = %q want list_dir", got)
	}
	if got := sessionToolCalled(path, from, "ccc"); got != "" {
		t.Errorf("unknown session returned %q", got)
	}
}

// doctor passes a model only when a tool actually ran. now that refusals are
// audited too, a model that only reached for a denied tool must still fail the
// health check rather than counting as "invoked a tool".
func TestSessionToolCalledIgnoresDenied(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	body := `{"ts":"t1","method":"tool.denied","params":{"session":"aaa","tool":"execute_shell_command","args":{"command":"curl"}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if got := sessionToolCalled(path, 0, "aaa"); got != "" {
		t.Errorf("sessionToolCalled = %q, want empty (only a denied call was logged)", got)
	}
}
