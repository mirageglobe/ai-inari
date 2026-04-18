// Package audit writes an append-only JSONL log of every tool-call the daemon handles.
// Each entry records the RPC method, params, and a timestamp for operator inspection.
package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Entry is a single audit log record.
type Entry struct {
	Timestamp time.Time       `json:"ts"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params,omitempty"`
}

// Auditor writes append-only JSON-lines audit entries to a file.
type Auditor struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

// New opens (or creates) the audit log at path. Panics on failure — a daemon that cannot
// write its audit log should not start silently, as tool calls would go unrecorded.
// 0600: the log may contain sensitive prompt content; restrict to the daemon owner.
func New(path string) *Auditor {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic("audit: " + err.Error())
	}
	return &Auditor{f: f, enc: json.NewEncoder(f)}
}

func (a *Auditor) Log(method string, params json.RawMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.enc.Encode(Entry{
		Timestamp: time.Now().UTC(),
		Method:    method,
		Params:    params,
	})
}

func (a *Auditor) Close() {
	a.f.Close()
}
