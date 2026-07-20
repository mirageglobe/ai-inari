package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAuditLogPath asserts doctor computes the same audit path the daemon writes:
// beside the session store, defaulting under inariDir() when cfg.DataDir is unset.
func TestAuditLogPath(t *testing.T) {
	// unset data dir -> default store dir's parent + inari-audit.log
	wantDefault := filepath.Join(inariDir(), "inari-audit.log")
	if got := auditLogPath(""); got != wantDefault {
		t.Errorf("auditLogPath(\"\") = %q, want %q", got, wantDefault)
	}
	// explicit data dir -> log sits beside it (its parent)
	if got := auditLogPath("/srv/inari/store"); got != "/srv/inari/inari-audit.log" {
		t.Errorf("auditLogPath(custom) = %q, want /srv/inari/inari-audit.log", got)
	}
}

// TestSessionToolCalled covers the audit-log scan: it finds the tool.call for the
// target session, ignores other methods and other sessions, honours the start
// offset, and returns "" when there is no match.
func TestSessionToolCalled(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	// a preamble the scan must skip via the offset, then this-turn entries.
	preamble := `{"ts":"2026-01-01T00:00:00Z","method":"tool.call","params":{"session":"OLD","tool":"read_file"}}` + "\n"
	if err := os.WriteFile(logPath, []byte(preamble), 0644); err != nil {
		t.Fatal(err)
	}
	from := fileSize(logPath) // scan only what is appended after here

	lines := "" +
		`{"ts":"2026-01-01T00:01:00Z","method":"session.stream","params":{"id":"S1"}}` + "\n" +
		`{"ts":"2026-01-01T00:01:01Z","method":"tool.call","params":{"session":"OTHER","tool":"grep_file"}}` + "\n" +
		`{"ts":"2026-01-01T00:01:02Z","method":"tool.call","params":{"session":"S1","tool":"list_dir","failed":false}}` + "\n"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(lines); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if got := sessionToolCalled(logPath, from, "S1"); got != "list_dir" {
		t.Errorf("sessionToolCalled(S1) = %q, want list_dir", got)
	}
	// offset must hide the preamble entry for OLD
	if got := sessionToolCalled(logPath, from, "OLD"); got != "" {
		t.Errorf("sessionToolCalled(OLD) = %q, want \"\" (before offset)", got)
	}
	// a session that never called a tool
	if got := sessionToolCalled(logPath, from, "S1-nope"); got != "" {
		t.Errorf("sessionToolCalled(miss) = %q, want \"\"", got)
	}
	// missing file is a clean empty result, not a panic
	if got := sessionToolCalled(filepath.Join(dir, "nope.log"), 0, "S1"); got != "" {
		t.Errorf("sessionToolCalled(missing file) = %q, want \"\"", got)
	}
}
