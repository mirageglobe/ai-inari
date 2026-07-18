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
