// probe.go owns the `inari probe` command: driving the fixture task suite through
// a real model and reporting which tool it selected for each task. it does NOT own
// the fixture or task list (probe_suite.go) or the scoring (probe_report.go).

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mirageglobe/ai-inari/internal/config"
	"github.com/mirageglobe/ai-inari/internal/ipc"
)

// runProbe audits the builtin tool surface against a real model. `doctor --models`
// asks "does this model call any tool at all"; probe asks the harder question the
// tool-surface review needs: for a task each builtin exists to serve, does the
// model pick that builtin, reach for the shell instead, or answer without tools.
// the corpus this generates is the input to schema/description tuning - before it
// existed there were no real session tool-call logs to audit.
func runProbe(args []string) {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	modelFlag := fs.String("model", "", "model tag to probe (default: config models.thinker)")
	runs := fs.Int("runs", 1, "repeat the suite N times; small models are stochastic, so a single run is a sample, not a verdict")
	cfgFlag := fs.String("config", "", "path to config.json")
	fs.Parse(args) //nolint:errcheck

	cfgPath := defaultConfigPath()
	if *cfgFlag != "" {
		cfgPath = *cfgFlag
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		line("fail", "config", err.Error())
		os.Exit(1)
	}
	model := strings.TrimSpace(*modelFlag)
	if model == "" {
		model = cfg.Models.Thinker
	}
	if model == "" {
		line("fail", "model", "no model given and models.thinker is empty")
		os.Exit(1)
	}

	fixture, err := os.MkdirTemp("", "inari-probe-*")
	if err != nil {
		line("fail", "fixture", err.Error())
		os.Exit(1)
	}
	defer os.RemoveAll(fixture)
	if err := writeProbeFixture(fixture); err != nil {
		line("fail", "fixture", err.Error())
		os.Exit(1)
	}

	ensureDaemon(cfgPath)
	client := ipc.NewClient(defaultSocket)
	defer client.Close()
	auditPath := auditLogPath(cfg.DataDir)

	fmt.Printf("inari probe %s  (%d task%s x %d run%s)\n\n", model, len(probeTasks), plural(len(probeTasks)), *runs, plural(*runs))
	var all []probeResult
	for run := 1; run <= *runs; run++ {
		if *runs > 1 {
			fmt.Printf("-- run %d --\n", run)
		}
		for _, task := range probeTasks {
			r := probeOne(client, model, fixture, auditPath, task)
			all = append(all, r)
			status := "fail"
			if r.verdict() == verdictHit {
				status = "ok"
			} else if r.verdict() == verdictMiss {
				status = "warn"
			}
			detail := fmt.Sprintf("want %-22s got %s", task.want, r.chain())
			if r.err != "" {
				detail += "  [" + r.err + "]"
			}
			line(status, task.name, detail)
		}
	}
	printProbeSummary(summariseProbe(all), len(all))
}

// probeOne runs a single task in its own throwaway session and returns every tool
// the model reached for. the session is deleted afterward so runs stay independent:
// a shared session would let one task's history few-shot the next one's selection.
func probeOne(client *ipc.Client, model, fixture, auditPath string, task probeTask) probeResult {
	from := fileSize(auditPath)
	res := probeResult{task: task}

	info, err := client.CreateSession("probe-"+task.name, fixture)
	if err != nil {
		res.err = "session: " + err.Error()
		return res
	}
	defer client.DeleteSession(info.ID)
	if err := client.AssignModel(info.ID, model); err != nil {
		res.err = "assign: " + err.Error()
		return res
	}

	tokens := make(chan string, 256)
	statuses := make(chan string, 32)
	toolReqs := make(chan ipc.ToolRequestMsg, 8)
	approvals := make(chan bool, 8)

	// anything that asks for approval is denied: the probe must never run
	// unreviewed commands. the refusal still shows up in the report because the
	// daemon audits denials too, so no client-side bookkeeping is needed here.
	done := make(chan struct{})
	go func() {
		for range toolReqs {
			approvals <- false
		}
		close(done)
	}()
	go drain(statuses)
	go drain(tokens) // must drain concurrently: ChatStream blocks on a full buffer

	streamErr := client.ChatStream(info.ID, task.prompt, tokens, statuses, toolReqs, approvals)
	close(tokens)
	close(statuses)
	close(toolReqs)
	<-done
	if streamErr != nil {
		res.err = "chat: " + streamErr.Error()
	}

	res.calls = sessionToolCalls(auditPath, from, info.ID)
	return res
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
