// audit_scan.go owns reading model tool-call records back out of the audit log.
// it does NOT own writing them (internal/ipc/stream.go) or deciding what they mean
// (doctor_models.go verdicts, probe_report.go summaries).

package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
)

// probeCall is one tool invocation observed during a turn: either executed
// ("tool.call") or refused by the user ("tool.denied"). both come from the audit
// log, so the observed chain is whatever the daemon recorded.
type probeCall struct {
	tool   string
	args   map[string]any
	failed bool
	denied bool
}

// sessionToolCalls scans the audit log from byte offset `from` and returns every
// tool record logged for sessionID, executed or denied, in log order. reads only
// new bytes so a long-lived log does not slow the scan.
func sessionToolCalls(auditPath string, from int64, sessionID string) []probeCall {
	f, err := os.Open(auditPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return nil
	}

	var calls []probeCall
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var e struct {
			Method string `json:"method"`
			Params struct {
				Session string         `json:"session"`
				Tool    string         `json:"tool"`
				Args    map[string]any `json:"args"`
				Failed  bool           `json:"failed"`
			} `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue // a partial or non-JSON line is noise, not a scan failure
		}
		if e.Params.Session != sessionID {
			continue
		}
		if e.Method != "tool.call" && e.Method != "tool.denied" {
			continue
		}
		calls = append(calls, probeCall{
			tool:   e.Params.Tool,
			args:   e.Params.Args,
			failed: e.Params.Failed,
			denied: e.Method == "tool.denied",
		})
	}
	return calls
}

// sessionToolCalled returns the name of the first tool that actually ran for
// sessionID, or "" if none: doctor's pass/fail signal. denied records are skipped
// on purpose - reaching for a refused tool is not proof the model can use tools.
func sessionToolCalled(auditPath string, from int64, sessionID string) string {
	for _, c := range sessionToolCalls(auditPath, from, sessionID) {
		if !c.denied {
			return c.tool
		}
	}
	return ""
}

// fileSize returns the byte length of path, or 0 if it does not exist yet.
func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}
