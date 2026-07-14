package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/mirageglobe/ai-inari/internal/config"
	"github.com/mirageglobe/ai-inari/internal/ollama"
)

// cmdDoctor checks inari's runtime dependencies and prints one status line per
// check. it exits non-zero when a required check fails (config, ollama, base
// model) so it can gate a preflight or CI step; daemon state and worker/sensor
// models are advisory and never fail the command.
func cmdDoctor(cfgPath string) {
	fmt.Println("inari doctor")
	fmt.Println()

	ok := true // flips to false on any required-check failure

	// config: report whether it already existed or was just created with defaults.
	existed := true
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		existed = false
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		line("fail", "config", fmt.Sprintf("%s: %v", cfgPath, err))
		os.Exit(1) // nothing else can run without config
	}
	if existed {
		line("ok", "config", cfgPath)
	} else {
		line("warn", "config", "created with defaults at "+cfgPath)
	}

	// ollama reachable at the configured base url.
	client := ollama.NewClient(cfg.OllamaBaseURL)
	ollamaUp := client.Ping() == nil
	if ollamaUp {
		line("ok", "ollama", cfg.OllamaBaseURL)
	} else {
		line("fail", "ollama", cfg.OllamaBaseURL+"  (hint: run `ollama serve`)")
		ok = false
	}

	// models: the thinker is the base model and is required to chat; worker and
	// sensor are advisory since not every setup uses all tiers.
	if ollamaUp {
		ok = checkModels(client, cfg.Models) && ok
	} else {
		line("warn", "models", "skipped (ollama unreachable)")
	}

	// ollama tuning: advisory. keep_alive is applied by inarid per request; the
	// other two are server-start settings the user must set on `ollama serve`.
	reportOllamaTuning(cfg.Ollama)

	// daemon: informational; "not running" is a valid state, not a failure.
	if pid, err := readPID(); err == nil && alive(pid) {
		line("ok", "daemon", fmt.Sprintf("running (pid %d)", pid))
		checkSocket(cfg.Socket)
	} else {
		line("ok", "daemon", "not running")
	}

	fmt.Println()
	if ok {
		fmt.Println("[ ok ] all required checks passed")
		return
	}
	fmt.Println("[fail] one or more required checks failed")
	os.Exit(1)
}

// checkModels verifies the configured base model is pulled and reports the
// worker/sensor tiers. returns false only when the required base model is absent.
func checkModels(client *ollama.Client, m config.Models) bool {
	installed, err := client.ListModels()
	if err != nil {
		line("warn", "models", "could not list: "+err.Error())
		return true // cannot prove absence; do not fail the run
	}
	names := make([]string, len(installed))
	for i, mdl := range installed {
		names[i] = mdl.Name
	}

	base := true
	if modelPresent(names, m.Thinker) {
		line("ok", "base model", m.Thinker)
	} else {
		line("fail", "base model", m.Thinker+"  (hint: run `ollama pull "+m.Thinker+"`)")
		base = false
	}

	// advisory tiers, in fixed order (map iteration order is not stable).
	for _, t := range []struct{ label, want string }{
		{"worker", m.Worker},
		{"sensor", m.Sensor},
	} {
		if t.want == "" {
			continue
		}
		if modelPresent(names, t.want) {
			line("ok", t.label, t.want)
		} else {
			line("warn", t.label, t.want+"  (not pulled)")
		}
	}
	return base
}

// reportOllamaTuning prints the configured Ollama runtime tuning. keep_alive is
// applied by inarid on each request; max_loaded_models / num_parallel cannot be
// set on an external `ollama serve`, so they are surfaced as the host env vars the
// user should export. lines are advisory and never fail the command.
func reportOllamaTuning(o config.Ollama) {
	if o.KeepAlive != "" {
		line("ok", "keep_alive", o.KeepAlive+"  (applied per request)")
	} else {
		line("ok", "keep_alive", "ollama default  (set ollama.keep_alive to override)")
	}
	if o.MaxLoadedModels > 0 {
		line("warn", "ollama env", fmt.Sprintf("export OLLAMA_MAX_LOADED_MODELS=%d on `ollama serve`", o.MaxLoadedModels))
	}
	if o.NumParallel > 0 {
		line("warn", "ollama env", fmt.Sprintf("export OLLAMA_NUM_PARALLEL=%d on `ollama serve`", o.NumParallel))
	}
}

// checkSocket reports the daemon socket exists with the expected 0600 mode; only
// meaningful while the daemon is up, so callers guard on that first.
func checkSocket(path string) {
	fi, err := os.Stat(path)
	if err != nil {
		line("warn", "socket", path+"  (missing)")
		return
	}
	if fi.Mode().Perm() == 0600 {
		line("ok", "socket", path)
	} else {
		line("warn", "socket", fmt.Sprintf("%s  (mode %o, want 600)", path, fi.Mode().Perm()))
	}
}

// line prints one aligned status row: "[ ok ] label:   detail".
func line(status, label, detail string) {
	tag := "[fail]"
	switch status {
	case "ok":
		tag = "[ ok ]"
	case "warn":
		tag = "[warn]"
	}
	fmt.Printf("%s %-12s %s\n", tag, label+":", detail)
}

// alive reports whether pid is a live process via a signal-0 probe.
func alive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// modelPresent reports whether want is among the installed ollama model names,
// tolerating a missing or ":latest" tag on either side.
func modelPresent(names []string, want string) bool {
	norm := func(s string) string { return strings.TrimSuffix(s, ":latest") }
	w := norm(want)
	for _, n := range names {
		if norm(n) == w {
			return true
		}
	}
	return false
}
