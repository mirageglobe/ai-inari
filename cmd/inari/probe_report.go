// probe_report.go owns scoring and rendering a probe run: turning observed tool
// calls into the audit signal (hit / miss / no call, shell fallthrough, unused
// builtins). it does NOT own running the tasks (probe.go) or the fixture
// (probe_suite.go).

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mirageglobe/inari/internal/ipc"
)

const (
	verdictHit  = "hit"  // the intended builtin was called
	verdictMiss = "miss" // a tool ran, but not the intended one
	verdictNone = "none" // the model answered without calling anything
)

// probeResult is one task's outcome: every tool the model reached for, in order.
type probeResult struct {
	task  probeTask
	calls []probeCall
	err   string
}

// verdict scores the result. any occurrence of the wanted tool counts as a hit:
// a model that stats a file before reading it has still selected correctly, and
// penalising the extra call would measure efficiency, not selection.
func (r probeResult) verdict() string {
	if len(r.calls) == 0 {
		return verdictNone
	}
	for _, c := range r.calls {
		if c.tool == r.task.want {
			return verdictHit
		}
	}
	return verdictMiss
}

// chain renders the observed calls as a compact "tool>tool" trail for the report,
// annotating shell calls with their binary (the fallthrough detail that matters)
// and marking denied or failed calls.
func (r probeResult) chain() string {
	if len(r.calls) == 0 {
		return "-"
	}
	parts := make([]string, len(r.calls))
	for i, c := range r.calls {
		s := c.tool
		if cmd := shellCommand(c); cmd != "" {
			s += "(" + cmd + ")"
		}
		switch {
		case c.denied:
			s += "!denied"
		case c.failed:
			s += "!failed"
		}
		parts[i] = s
	}
	return strings.Join(parts, " > ")
}

// shellCommand returns the binary an execute_shell_command call asked for, or ""
// for any other tool.
func shellCommand(c probeCall) string {
	if c.tool != "execute_shell_command" {
		return ""
	}
	cmd, _ := c.args["command"].(string)
	return cmd
}

// probeSummary is the run-level audit signal the roadmap item asks for.
type probeSummary struct {
	hits, misses, noCall int
	used                 map[string]int // tool -> times called across the run
	unused               []string       // declared builtins the model never called
	fellThrough          []string       // tasks that reached for shell instead of a builtin
	shellCmds            map[string]int // shell binary -> times requested
}

// summariseProbe aggregates results into the run-level signal. unused is computed
// against ipc.BuiltinToolNames so adding a builtin automatically widens the audit.
func summariseProbe(results []probeResult) probeSummary {
	s := probeSummary{used: map[string]int{}, shellCmds: map[string]int{}}
	for _, r := range results {
		switch r.verdict() {
		case verdictHit:
			s.hits++
		case verdictMiss:
			s.misses++
		default:
			s.noCall++
		}
		shellUsed := false
		for _, c := range r.calls {
			s.used[c.tool]++
			if cmd := shellCommand(c); cmd != "" {
				s.shellCmds[cmd]++
				shellUsed = true
			}
		}
		// a task that legitimately wants shell is not a fallthrough; only a
		// builtin-shaped task answered with shell is.
		if shellUsed && r.task.want != "execute_shell_command" {
			s.fellThrough = append(s.fellThrough, r.task.name)
		}
	}
	for _, name := range ipc.BuiltinToolNames() {
		if s.used[name] == 0 {
			s.unused = append(s.unused, name)
		}
	}
	return s
}

// printProbeSummary writes the run-level findings under the per-task lines.
func printProbeSummary(s probeSummary, tasks int) {
	fmt.Printf("\nselection  %d/%d hit, %d miss, %d no-call\n", s.hits, tasks, s.misses, s.noCall)
	if len(s.unused) > 0 {
		fmt.Printf("unused     %s\n", strings.Join(s.unused, ", "))
	} else {
		fmt.Println("unused     none (every builtin was selected at least once)")
	}
	if len(s.fellThrough) > 0 {
		fmt.Printf("fell to sh %s\n", strings.Join(s.fellThrough, ", "))
	}
	if len(s.shellCmds) > 0 {
		cmds := make([]string, 0, len(s.shellCmds))
		for c, n := range s.shellCmds {
			cmds = append(cmds, fmt.Sprintf("%s x%d", c, n))
		}
		sort.Strings(cmds)
		fmt.Printf("shell ran  %s\n", strings.Join(cmds, ", "))
	}
}
