package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
)

func TestLog(t *testing.T) {
	f, err := os.CreateTemp("", "inari-audit-*.log")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)

	a := New(path)
	a.Log("ping", json.RawMessage(`{"hello":"world"}`))
	a.Close()

	f, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var entry Entry
	if err := json.NewDecoder(bufio.NewReader(f)).Decode(&entry); err != nil {
		t.Fatalf("decode entry: %v", err)
	}
	if entry.Method != "ping" {
		t.Errorf("method = %q, want ping", entry.Method)
	}
	if entry.Timestamp.IsZero() {
		t.Error("timestamp is zero")
	}
}
