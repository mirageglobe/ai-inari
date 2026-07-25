package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mirageglobe/ai-inari/internal/config"
	"github.com/mirageglobe/ai-inari/internal/ipc"
)

// verifyModels drives each configured, locally-present model through one real
// streaming turn against a fixture cwd and confirms it (1) replies without error
// and (2) actually invokes a tool. presence is not function: a pulled model can
// still fail to load or never tool-call. tool use is confirmed via the audit log,
// not the reply text - a model can narrate a tool result without one having run
// (the failure the "surface tool output on empty final answer" fix addressed).
// returns false if any configured+present model fails; absent models are already
// reported by checkModels and skipped here (scope: only what is on this machine).
func verifyModels(cfgPath string, cfg *config.Config, installed []string) bool {
	fixture, err := os.MkdirTemp("", "inari-doctor-*")
	if err != nil {
		line("warn", "verify", "fixture: "+err.Error())
		return true // cannot set up the test; do not fail the run
	}
	defer os.RemoveAll(fixture)
	if err := os.WriteFile(filepath.Join(fixture, "marker.txt"), []byte("inari\n"), 0644); err != nil {
		line("warn", "verify", "fixture: "+err.Error())
		return true
	}

	ensureDaemon(cfgPath)
	client := ipc.NewClient(defaultSocket)
	defer client.Close()
	auditPath := auditLogPath(cfg.DataDir)

	targets := []struct{ role, tag string }{{"thinker", cfg.Models.Thinker}}
	if cfg.Models.Runner != "" {
		targets = append(targets, struct{ role, tag string }{"runner", cfg.Models.Runner})
	}

	allOK := true
	for _, t := range targets {
		if strings.TrimSpace(t.tag) == "" {
			continue
		}
		if !modelPresent(installed, t.tag) {
			continue // not pulled: checkModels already flagged it; nothing to run
		}
		ok, detail := verifyOne(client, t.tag, fixture, auditPath)
		if ok {
			line("ok", "verify "+t.role, t.tag+"  ("+detail+")")
		} else {
			line("fail", "verify "+t.role, t.tag+"  ("+detail+")")
			allOK = false
		}
	}
	return allOK
}

// verifyOne runs a single fixture turn for tag and returns whether the model
// replied error-free AND a tool.call for the fresh session landed in the audit
// log. the session is deleted afterward so repeated doctor runs do not pile up.
func verifyOne(client *ipc.Client, tag, fixture, auditPath string) (bool, string) {
	// only scan audit entries this turn appends (the log is append-only, shared
	// across the daemon's whole lifetime).
	from := fileSize(auditPath)

	info, err := client.CreateSession("doctor-"+tag, fixture)
	if err != nil {
		return false, "session: " + err.Error()
	}
	defer client.DeleteSession(info.ID)
	if err := client.AssignModel(info.ID, tag); err != nil {
		return false, "assign: " + err.Error()
	}

	tokens := make(chan string, 256)
	statuses := make(chan string, 32)
	toolReqs := make(chan ipc.ToolRequestMsg, 8)
	approvals := make(chan bool, 8)
	// safe builtins (list_dir/read_file) auto-execute server-side and never emit a
	// tool_request. deny anything that does ask, so a model reaching for shell can
	// neither hang the check nor run commands during a health check.
	go func() {
		for range toolReqs {
			approvals <- false
		}
	}()
	go drain(statuses)
	go drain(tokens) // must drain concurrently: ChatStream blocks on a full buffer

	streamErr := client.ChatStream(info.ID, "list the files in this directory", tokens, statuses, toolReqs, approvals)
	// ChatStream has returned, so no more sends; closing ends the drain goroutines.
	close(tokens)
	close(statuses)
	close(toolReqs)
	if streamErr != nil {
		return false, "chat: " + streamErr.Error()
	}

	if tool := sessionToolCalled(auditPath, from, info.ID); tool != "" {
		return true, "replied + called " + tool
	}
	return false, "replied but invoked no tool"
}

// drain empties a channel until it is closed; used to keep ChatStream unblocked
// while its output is not otherwise consumed.
func drain[T any](ch <-chan T) {
	for range ch {
	}
}

// the audit scan (sessionToolCalls/sessionToolCalled) and fileSize live in
// audit_scan.go, shared with the tool-surface probe.
