package main

import (
	"bytes"
	"strings"
	"testing"
)

// resolveMessage is the headless message-source resolver: flag text, or stdin on "-".
func TestResolveMessage(t *testing.T) {
	tests := []struct {
		name    string
		flagVal string
		stdin   string
		want    string
		wantErr bool
	}{
		{"flag text", "hello", "", "hello", false},
		{"empty flag", "", "", "", true},
		{"whitespace flag", "   ", "", "", true},
		{"stdin dash", "-", "from stdin\n", "from stdin", false},
		{"stdin multiline", "-", "line one\nline two\n", "line one\nline two", false},
		{"stdin empty", "-", "   \n", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveMessage(tc.flagVal, strings.NewReader(tc.stdin))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// printReply renders plain text by default and a JSON object under --json.
func TestPrintReply(t *testing.T) {
	var buf bytes.Buffer

	printReply(&buf, "hi there", false)
	if buf.String() != "hi there\n" {
		t.Errorf("plain: got %q, want %q", buf.String(), "hi there\n")
	}

	buf.Reset()
	printReply(&buf, "hi there", true)
	if got := strings.TrimSpace(buf.String()); got != `{"reply":"hi there"}` {
		t.Errorf("json: got %q, want %q", got, `{"reply":"hi there"}`)
	}
}

// resolveTarget requires exactly one of --new / --session.
func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name    string
		isNew   bool
		session string
		wantErr bool
	}{
		{"session only", false, "abc123", false},
		{"new only", true, "", false},
		{"neither", false, "", true},
		{"both", true, "abc123", true},
		{"new + blank session", true, "  ", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := resolveTarget(tc.isNew, tc.session); (err != nil) != tc.wantErr {
				t.Fatalf("resolveTarget(%v,%q) err=%v, wantErr %v", tc.isNew, tc.session, err, tc.wantErr)
			}
		})
	}
}

// newSessionName returns the given name, trimmed, or a generated headless-* default.
func TestNewSessionName(t *testing.T) {
	if got := newSessionName("my session"); got != "my session" {
		t.Errorf("provided name: got %q, want %q", got, "my session")
	}
	if got := newSessionName("  spaced  "); got != "spaced" {
		t.Errorf("trims: got %q, want %q", got, "spaced")
	}
	if got := newSessionName(""); !strings.HasPrefix(got, "headless-") {
		t.Errorf("default: got %q, want a headless-* prefix", got)
	}
}
